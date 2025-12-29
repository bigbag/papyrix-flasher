package flasher

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/bigbag/papyrix-flasher/internal/protocol"
	"github.com/bigbag/papyrix-flasher/internal/serial"
	"github.com/bigbag/papyrix-flasher/internal/slip"
)

// ProgressCallback is called to report flash progress.
type ProgressCallback func(current, total int)

// Flasher handles flashing firmware to ESP32 devices.
type Flasher struct {
	port     *serial.Port
	progress ProgressCallback
}

// New creates a new Flasher for the given port.
func New(port *serial.Port) *Flasher {
	return &Flasher{port: port}
}

// SetProgressCallback sets the progress callback function.
func (f *Flasher) SetProgressCallback(cb ProgressCallback) {
	f.progress = cb
}

// reportProgress calls the progress callback if set.
func (f *Flasher) reportProgress(current, total int) {
	if f.progress != nil {
		f.progress(current, total)
	}
}

// Connect establishes connection with the bootloader.
func (f *Flasher) Connect() error {
	// Reset into bootloader
	if err := f.port.ResetToBootloader(); err != nil {
		return fmt.Errorf("failed to reset into bootloader: %w", err)
	}

	// Sync with bootloader
	if err := f.sync(); err != nil {
		return fmt.Errorf("failed to sync with bootloader: %w", err)
	}

	// Attach SPI flash
	if err := f.spiAttach(); err != nil {
		return fmt.Errorf("failed to attach SPI flash: %w", err)
	}

	return nil
}

// sync sends the SYNC command to establish communication.
func (f *Flasher) sync() error {
	syncReq := protocol.NewRequest(protocol.CmdSync, protocol.SyncData())
	frame := slip.Encode(syncReq.Encode())

	for attempt := 0; attempt < 10; attempt++ {
		f.port.Flush()

		if _, err := f.port.Write(frame); err != nil {
			continue
		}

		resp, err := f.readResponse(500 * time.Millisecond)
		if err != nil {
			continue
		}

		if resp.Command == protocol.CmdSync && resp.IsSuccess() {
			// Drain any additional sync responses
			for i := 0; i < 7; i++ {
				f.readResponse(100 * time.Millisecond)
			}
			return nil
		}
	}

	return fmt.Errorf("sync failed after 10 attempts")
}

// spiAttach attaches the SPI flash.
func (f *Flasher) spiAttach() error {
	req := protocol.NewRequest(protocol.CmdSpiAttach, protocol.SpiAttachData())
	return f.sendCommand(req)
}

// FlashImage flashes a binary image to the specified address.
func (f *Flasher) FlashImage(data []byte, address uint32, verify bool) error {
	size := uint32(len(data))
	numBlocks := protocol.CalculateFlashBlocks(len(data))
	eraseSize := protocol.CalculateEraseSize(len(data))

	// Send FLASH_BEGIN
	beginData := protocol.FlashBeginData(eraseSize, numBlocks, protocol.FlashBlockSize, address)
	beginReq := protocol.NewRequest(protocol.CmdFlashBegin, beginData)
	if err := f.sendCommand(beginReq); err != nil {
		return fmt.Errorf("flash begin failed: %w", err)
	}

	// Send data blocks
	blockSize := protocol.FlashBlockSize
	totalBlocks := int(numBlocks)

	for seq := 0; seq < totalBlocks; seq++ {
		start := seq * blockSize
		end := start + blockSize
		if end > len(data) {
			end = len(data)
		}

		block := data[start:end]
		blockData := protocol.FlashDataData(block, uint32(seq))
		blockReq := protocol.NewRequest(protocol.CmdFlashData, blockData)

		if err := f.sendCommand(blockReq); err != nil {
			return fmt.Errorf("flash data block %d failed: %w", seq, err)
		}

		f.reportProgress(seq+1, totalBlocks)
	}

	// Send FLASH_END (don't reboot yet)
	endData := protocol.FlashEndData(false)
	endReq := protocol.NewRequest(protocol.CmdFlashEnd, endData)
	if err := f.sendCommand(endReq); err != nil {
		return fmt.Errorf("flash end failed: %w", err)
	}

	// Verify if requested
	if verify {
		if err := f.verifyFlash(data, address, size); err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}
	}

	return nil
}

// verifyFlash verifies the flashed data using MD5.
func (f *Flasher) verifyFlash(data []byte, address, size uint32) error {
	// Calculate expected MD5
	hash := md5.Sum(data)
	expected := hex.EncodeToString(hash[:])

	// Request MD5 from device
	md5Data := protocol.FlashMD5Data(address, size)
	req := protocol.NewRequest(protocol.CmdSpiFlashMD5, md5Data)
	frame := slip.Encode(req.Encode())

	if _, err := f.port.Write(frame); err != nil {
		return err
	}

	resp, err := f.readResponse(10 * time.Second) // MD5 can take a while for large images
	if err != nil {
		return err
	}

	if !resp.IsSuccess() {
		return fmt.Errorf("MD5 command failed: %s", resp.ErrorString())
	}

	// Response data contains the MD5 hash as ASCII hex
	actual := string(resp.Data)
	if len(actual) >= 32 {
		actual = actual[:32]
	}

	if actual != expected {
		return fmt.Errorf("MD5 mismatch: expected %s, got %s", expected, actual)
	}

	return nil
}

// Reboot reboots the device.
func (f *Flasher) Reboot() error {
	// Send FLASH_END with reboot flag
	endData := protocol.FlashEndData(true)
	endReq := protocol.NewRequest(protocol.CmdFlashEnd, endData)
	frame := slip.Encode(endReq.Encode())

	_, err := f.port.Write(frame)
	if err != nil {
		return err
	}

	// Also do a hard reset via DTR/RTS
	time.Sleep(100 * time.Millisecond)
	return f.port.HardReset()
}

// sendCommand sends a command and waits for successful response.
func (f *Flasher) sendCommand(req *protocol.Request) error {
	frame := slip.Encode(req.Encode())

	if _, err := f.port.Write(frame); err != nil {
		return err
	}

	resp, err := f.readResponse(5 * time.Second)
	if err != nil {
		return err
	}

	if !resp.IsSuccess() {
		return fmt.Errorf("command 0x%02X failed: %s", req.Command, resp.ErrorString())
	}

	return nil
}

// readResponse reads and decodes a response from the bootloader.
func (f *Flasher) readResponse(timeout time.Duration) (*protocol.Response, error) {
	deadline := time.Now().Add(timeout)
	var buffer []byte

	for time.Now().Before(deadline) {
		chunk := make([]byte, 256)
		n, err := f.port.ReadWithTimeout(chunk, 100*time.Millisecond)
		if n > 0 {
			buffer = append(buffer, chunk[:n]...)
		}
		if err != nil && n == 0 {
			continue
		}

		// Try to extract a frame
		frame, remaining := slip.ReadFrame(buffer)
		if frame != nil {
			buffer = remaining
			data := slip.Decode(frame)
			if len(data) >= 10 {
				return protocol.DecodeResponse(data)
			}
		}
	}

	return nil, fmt.Errorf("timeout waiting for response")
}

// FlashRegion represents a region to flash.
type FlashRegion struct {
	Address uint32
	Data    []byte
	Name    string
}

// FlashMultiple flashes multiple regions in sequence.
func (f *Flasher) FlashMultiple(regions []FlashRegion, verify bool) error {
	totalSize := 0
	for _, r := range regions {
		totalSize += len(r.Data)
	}

	currentProgress := 0
	for _, region := range regions {
		blocks := int(protocol.CalculateFlashBlocks(len(region.Data)))

		// Set up progress callback for this region
		f.SetProgressCallback(func(current, total int) {
			f.reportProgress(currentProgress+current, totalSize/protocol.FlashBlockSize)
		})

		if err := f.FlashImage(region.Data, region.Address, verify); err != nil {
			return fmt.Errorf("failed to flash %s at 0x%X: %w", region.Name, region.Address, err)
		}

		currentProgress += blocks
	}

	return nil
}
