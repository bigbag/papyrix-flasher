package flasher

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"time"

	"github.com/bigbag/papyrix-flasher/internal/protocol"
	"github.com/bigbag/papyrix-flasher/internal/serial"
	"github.com/bigbag/papyrix-flasher/internal/slip"
)

// Flasher handles flashing firmware to ESP32 devices.
type Flasher struct {
	port *serial.Port
}

// FlashRegion represents a region to flash.
type FlashRegion struct {
	Address uint32
	Data    []byte
	Name    string
}

// New creates a new Flasher for the given port.
func New(port *serial.Port) *Flasher {
	return &Flasher{port: port}
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

	// Set flash parameters (16MB flash)
	if err := f.spiSetParams(16 * 1024 * 1024); err != nil {
		return fmt.Errorf("failed to set flash params: %w", err)
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

// spiSetParams sets flash parameters.
func (f *Flasher) spiSetParams(flashSize uint32) error {
	req := protocol.NewRequest(protocol.CmdSpiSetParams, protocol.SpiSetParamsData(flashSize))
	return f.sendCommand(req)
}

// FlashImageCompressed flashes a binary image using deflate compression.
func (f *Flasher) FlashImageCompressed(data []byte, address uint32, verify bool) error {
	// Compress the data using zlib
	var compressed bytes.Buffer
	writer, err := zlib.NewWriterLevel(&compressed, zlib.BestSpeed)
	if err != nil {
		return fmt.Errorf("failed to create zlib writer: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("failed to compress data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to finalize compression: %w", err)
	}

	compressedData := compressed.Bytes()
	compressionRatio := float64(len(data)) / float64(len(compressedData))
	fmt.Printf("Compressed %d -> %d bytes (%.1fx compression)\n", len(data), len(compressedData), compressionRatio)

	// Calculate blocks for compressed data
	blockSize := protocol.FlashBlockSize
	numBlocks := protocol.CalculateDeflBlocks(len(compressedData), blockSize)

	// Calculate erase size (based on uncompressed size, rounded to sector)
	eraseSize := protocol.CalculateEraseSize(len(data))

	// Send FLASH_DEFL_BEGIN
	beginData := protocol.FlashDeflBeginData(eraseSize, numBlocks, uint32(blockSize), address)
	beginReq := protocol.NewRequest(protocol.CmdFlashDeflBegin, beginData)

	// Calculate erase timeout based on uncompressed size
	eraseTimeout := time.Duration(eraseSize/1024/1024*3+5) * time.Second
	if err := f.sendCommandWithTimeout(beginReq, eraseTimeout); err != nil {
		return fmt.Errorf("flash defl begin failed: %w", err)
	}

	// Send compressed data blocks
	totalBlocks := int(numBlocks)
	for seq := 0; seq < totalBlocks; seq++ {
		start := seq * blockSize
		end := start + blockSize
		if end > len(compressedData) {
			end = len(compressedData)
		}

		block := compressedData[start:end]
		blockData := protocol.FlashDeflDataData(block, uint32(seq))
		blockReq := protocol.NewRequest(protocol.CmdFlashDeflData, blockData)

		// Retry up to 3 times on timeout
		var sendErr error
		for attempt := 0; attempt < 3; attempt++ {
			sendErr = f.sendCommand(blockReq)
			if sendErr == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
			f.port.Flush()
		}
		if sendErr != nil {
			return fmt.Errorf("flash defl data block %d failed: %w", seq, sendErr)
		}
	}

	// Send FLASH_DEFL_END - don't wait too long as device might reset
	endData := protocol.FlashDeflEndData(false)
	endReq := protocol.NewRequest(protocol.CmdFlashDeflEnd, endData)
	frame := slip.Encode(endReq.Encode())
	if _, err := f.port.Write(frame); err != nil {
		fmt.Printf("Warning: flash end write error (may be normal): %v\n", err)
	}
	// Try to read response but don't fail if it times out
	if _, err := f.readResponse(2 * time.Second); err != nil {
		fmt.Printf("Warning: flash end response timeout (may be normal): %v\n", err)
	}

	return nil
}

// Reboot reboots the device.
func (f *Flasher) Reboot() error {
	endData := protocol.FlashEndData(true)
	endReq := protocol.NewRequest(protocol.CmdFlashEnd, endData)
	frame := slip.Encode(endReq.Encode())

	_, err := f.port.Write(frame)
	if err != nil {
		return err
	}

	time.Sleep(100 * time.Millisecond)
	return f.port.HardReset()
}

// sendCommand sends a command and waits for successful response.
func (f *Flasher) sendCommand(req *protocol.Request) error {
	return f.sendCommandWithTimeout(req, 5*time.Second)
}

// sendCommandWithTimeout sends a command with a specific timeout.
func (f *Flasher) sendCommandWithTimeout(req *protocol.Request, timeout time.Duration) error {
	frame := slip.Encode(req.Encode())

	if _, err := f.port.Write(frame); err != nil {
		return err
	}

	resp, err := f.readResponse(timeout)
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
