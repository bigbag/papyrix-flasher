package flasher

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"math/bits"
	"time"

	"github.com/bigbag/papyrix-flasher/internal/protocol"
	"github.com/bigbag/papyrix-flasher/internal/serial"
	"github.com/bigbag/papyrix-flasher/internal/slip"
	"github.com/bigbag/papyrix-flasher/internal/stub"
)

// Flasher handles flashing firmware to ESP32 devices.
type Flasher struct {
	port        *serial.Port
	stubRunning bool
	buf         []byte   // shared buffer for SLIP frame assembly across reads
	pending     [][]byte // decoded non-response frames saved by readResponse
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

// Identify resets the device, synchronizes with the ROM bootloader, and reads
// its chip identity. It does not write registers, RAM, or flash.
func (f *Flasher) Identify() (*protocol.SecurityInfo, error) {
	f.stubRunning = false
	f.buf = nil
	f.pending = nil

	if err := f.port.ResetToBootloader(); err != nil {
		return nil, fmt.Errorf("failed to reset into bootloader: %w", err)
	}
	if err := f.sync(); err != nil {
		return nil, fmt.Errorf("failed to sync with bootloader: %w", err)
	}

	req := protocol.NewRequest(protocol.CmdGetSecurityInfo, nil)
	frame, err := encodeFrame(req)
	if err != nil {
		return nil, err
	}
	if _, err := f.port.Write(frame); err != nil {
		return nil, fmt.Errorf("failed to request chip identity: %w", err)
	}
	resp, err := f.readResponse(protocol.CmdGetSecurityInfo, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to read chip identity: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("get security info failed: %s", resp.ErrorString())
	}
	return protocol.ParseSecurityInfo(resp.Data)
}

// Connect identifies the ROM, checks that it matches expectedChipID, disables
// USB watchdogs when required, and starts the matching RAM stub.
func (f *Flasher) Connect(expectedChipID uint32) error {
	info, err := f.Identify()
	if err != nil {
		return err
	}
	if err := validateTarget(info, expectedChipID); err != nil {
		return err
	}
	if err := f.disableWatchdogs(info.ChipID); err != nil {
		return fmt.Errorf("failed to disable watchdogs: %w", err)
	}
	if err := f.uploadStub(info.ChipID); err != nil {
		return fmt.Errorf("failed to upload stub flasher: %w", err)
	}
	return nil
}

// sync sends the SYNC command to establish communication.
func (f *Flasher) sync() error {
	syncReq := protocol.NewRequest(protocol.CmdSync, protocol.SyncData())
	frame, err := encodeFrame(syncReq)
	if err != nil {
		return err
	}

	for range 10 {
		if err := f.port.Flush(); err != nil {
			continue
		}
		f.buf = nil

		if _, err := f.port.Write(frame); err != nil {
			continue
		}

		resp, err := f.readResponse(protocol.CmdSync, 500*time.Millisecond)
		if err != nil {
			continue
		}

		if resp.Command == protocol.CmdSync && resp.IsSuccess() {
			// Drain any additional sync responses
			for range 7 {
				if _, err := f.readResponse(protocol.CmdSync, 100*time.Millisecond); err != nil {
					break
				}
			}
			return nil
		}
	}

	return fmt.Errorf("sync failed after 10 attempts")
}

// readReg reads a 32-bit register on the target.
func (f *Flasher) readReg(addr uint32) (uint32, error) {
	req := protocol.NewRequest(protocol.CmdReadReg, protocol.ReadRegData(addr))
	frame, err := encodeFrame(req)
	if err != nil {
		return 0, err
	}

	if _, err := f.port.Write(frame); err != nil {
		return 0, err
	}

	resp, err := f.readResponse(protocol.CmdReadReg, 5*time.Second)
	if err != nil {
		return 0, err
	}

	if !resp.IsSuccess() {
		return 0, fmt.Errorf("read reg 0x%08X failed: %s", addr, resp.ErrorString())
	}

	return resp.Value, nil
}

// writeReg writes a 32-bit register on the target.
func (f *Flasher) writeReg(addr, value, mask, delayUs uint32) error {
	req := protocol.NewRequest(protocol.CmdWriteReg, protocol.WriteRegData(addr, value, mask, delayUs))
	return f.sendCommand(req)
}

type watchdogConfig struct {
	uartdevBufNo uint32
	usbJTAGPort  uint32
	wdtConfig0   uint32
	wdtWprotect  uint32
	swdConf      uint32
	swdWprotect  uint32
}

func configForChip(chipID uint32) (watchdogConfig, error) {
	switch chipID {
	case protocol.ChipIDESP32C3:
		return watchdogConfig{0x3FCDF07C, 3, 0x60008090, 0x600080A8, 0x600080AC, 0x600080B0}, nil
	case protocol.ChipIDESP32S3:
		return watchdogConfig{0x3FCEF14C, 4, 0x60008098, 0x600080B0, 0x600080B4, 0x600080B8}, nil
	default:
		return watchdogConfig{}, fmt.Errorf("unsupported device chip ID: 0x%02X", chipID)
	}
}

func validateTarget(info *protocol.SecurityInfo, expectedChipID uint32) error {
	switch info.ChipID {
	case protocol.ChipIDESP32C3, protocol.ChipIDESP32S3:
	default:
		return fmt.Errorf("unsupported device chip ID: 0x%02X", info.ChipID)
	}
	if info.ChipID != expectedChipID {
		return fmt.Errorf("firmware targets %s, but device is %s",
			protocol.ChipName(expectedChipID), protocol.ChipName(info.ChipID))
	}
	if info.Flags&0x5 != 0 {
		return fmt.Errorf("secure boot or secure download mode is enabled")
	}
	if bits.OnesCount8(info.FlashCryptCnt)%2 != 0 {
		return fmt.Errorf("flash encryption is enabled")
	}
	return nil
}

// disableWatchdogs disables RTC WDT and auto-feeds SWD when USB
// Serial/JTAG is active.
func (f *Flasher) disableWatchdogs(chipID uint32) error {
	config, err := configForChip(chipID)
	if err != nil {
		return err
	}
	uartNo, err := f.readReg(config.uartdevBufNo)
	if err != nil {
		if chipID == protocol.ChipIDESP32S3 {
			return fmt.Errorf("failed to read active ROM port: %w", err)
		}
		return nil
	}
	if chipID == protocol.ChipIDESP32S3 && uartNo&0xFF == 3 {
		return fmt.Errorf("ESP32-S3 USB-OTG transport is not supported; use USB Serial/JTAG")
	}
	if uartNo&0xFF != config.usbJTAGPort {
		return nil
	}

	if err := f.writeReg(config.wdtWprotect, protocol.RTCCntlWdtWkey, 0xFFFFFFFF, 0); err != nil {
		return err
	}
	if err := f.writeReg(config.wdtConfig0, 0, 0xFFFFFFFF, 0); err != nil {
		return err
	}
	if err := f.writeReg(config.wdtWprotect, 0, 0xFFFFFFFF, 0); err != nil {
		return err
	}
	if err := f.writeReg(config.swdWprotect, protocol.RTCCntlSwdWkey, 0xFFFFFFFF, 0); err != nil {
		return err
	}
	swdConf, err := f.readReg(config.swdConf)
	if err != nil {
		return err
	}
	if err := f.writeReg(config.swdConf, swdConf|protocol.RTCCntlSwdAutoFeedEn, 0xFFFFFFFF, 0); err != nil {
		return err
	}
	return f.writeReg(config.swdWprotect, 0, 0xFFFFFFFF, 0)
}

// uploadStub uploads the stub flasher to RAM and executes it.
func (f *Flasher) uploadStub(chipID uint32) error {
	s, err := stub.Get(chipID)
	if err != nil {
		return err
	}

	fmt.Println("Uploading stub flasher...")

	// Upload text segment
	if err := f.writeMemSegment(s.Text, s.TextStart); err != nil {
		return fmt.Errorf("text segment upload failed: %w", err)
	}

	// Upload data segment
	if len(s.Data) > 0 {
		if err := f.writeMemSegment(s.Data, s.DataStart); err != nil {
			return fmt.Errorf("data segment upload failed: %w", err)
		}
	}

	// Execute stub (executeFlag=0 means execute at entrypoint).
	// Send MEM_END and try to read response with short timeout (matches
	// esptool's MEM_END_ROM_TIMEOUT=0.2s). The ROM may not respond before
	// the stub takes over, so ignore errors.
	fmt.Println("Running stub flasher...")
	endData := protocol.MemEndData(0, s.Entry)
	endReq := protocol.NewRequest(protocol.CmdMemEnd, endData)
	frame, err := encodeFrame(endReq)
	if err != nil {
		return err
	}
	if _, err := f.port.Write(frame); err != nil {
		return fmt.Errorf("mem end write failed: %w", err)
	}
	if _, err := f.readResponse(protocol.CmdMemEnd, 200*time.Millisecond); err != nil {
		// The ROM may stop before it sends MEM_END. The stub greeting is the success check.
	}

	// Wait for "OHAI" greeting from stub (arrives as a raw SLIP packet)
	if err := f.waitForOHAI(); err != nil {
		return err
	}

	f.stubRunning = true
	fmt.Println("Stub running!")
	return nil
}

// writeMemSegment uploads a single memory segment via MEM_BEGIN/MEM_DATA.
func (f *Flasher) writeMemSegment(data []byte, offset uint32) error {
	blockSize := protocol.MemBlockSize
	numBlocks := (len(data) + blockSize - 1) / blockSize
	total, err := protocol.Uint32Len(len(data))
	if err != nil {
		return err
	}
	blocks, err := protocol.Uint32Len(numBlocks)
	if err != nil {
		return err
	}

	beginData := protocol.MemBeginData(total, blocks, uint32(blockSize), offset)
	beginReq := protocol.NewRequest(protocol.CmdMemBegin, beginData)
	if err := f.sendCommand(beginReq); err != nil {
		return err
	}

	for seq := 0; seq < numBlocks; seq++ {
		start := seq * blockSize
		end := start + blockSize
		if end > len(data) {
			end = len(data)
		}

		block := data[start:end]
		blockData, err := protocol.MemDataData(block, uint32(seq))
		if err != nil {
			return err
		}
		blockReq := protocol.NewDataRequest(protocol.CmdMemData, blockData, block)
		if err := f.sendCommand(blockReq); err != nil {
			return fmt.Errorf("block %d failed: %w", seq, err)
		}
	}

	return nil
}

// waitForOHAI waits for the stub's "OHAI" greeting (sent as a SLIP packet).
// Checks frames saved by readResponse first, then reads from the port.
func (f *Flasher) waitForOHAI() error {
	// Check frames already captured by readResponse
	for i, data := range f.pending {
		if bytes.Equal(data, []byte("OHAI")) {
			f.pending = append(f.pending[:i], f.pending[i+1:]...)
			return nil
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	chunk := make([]byte, 256)

	for time.Now().Before(deadline) {
		// Check buffer for any complete frames
		for {
			frame, remaining := slip.ReadFrame(f.buf)
			if frame == nil {
				break
			}
			f.buf = remaining
			data := slip.Decode(frame)
			if bytes.Equal(data, []byte("OHAI")) {
				return nil
			}
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		readTimeout := 500 * time.Millisecond
		if remaining < readTimeout {
			readTimeout = remaining
		}
		n, _ := f.port.ReadWithTimeout(chunk, readTimeout)
		if n > 0 {
			f.buf = append(f.buf, chunk[:n]...)
		}
	}
	return fmt.Errorf("timeout waiting for stub greeting (OHAI)")
}

// FlashImageCompressed flashes a binary image using deflate compression.
func (f *Flasher) FlashImageCompressed(data []byte, address uint32) error {
	// Compress the data using zlib (level 9, matches esptool)
	var compressed bytes.Buffer
	writer, err := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
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

	// Use larger blocks when stub is running
	blockSize := protocol.FlashBlockSize
	if f.stubRunning {
		blockSize = protocol.StubFlashWriteSize
	}
	numBlocks, err := protocol.CalculateDeflBlocks(len(compressedData), blockSize)
	if err != nil {
		return err
	}

	// The stub expects uncompressed size as the first param and erases as it writes.
	// The ROM bootloader expects the erase size rounded to sector boundary.
	eraseSize, err := protocol.Uint32Len(len(data))
	if err != nil {
		return err
	}
	if !f.stubRunning {
		eraseSize, err = protocol.CalculateEraseSize(len(data))
		if err != nil {
			return err
		}
	}

	// Send FLASH_DEFL_BEGIN
	beginData := protocol.FlashDeflBeginData(eraseSize, numBlocks, uint32(blockSize), address)
	beginReq := protocol.NewRequest(protocol.CmdFlashDeflBegin, beginData)

	// Calculate erase timeout based on uncompressed size
	eraseTimeout := time.Duration(eraseSize/1024/1024*3+5) * time.Second
	if err := f.sendCommandWithTimeout(beginReq, eraseTimeout); err != nil {
		return fmt.Errorf("flash defl begin failed: %w", err)
	}

	// Send compressed data blocks with progress
	totalBlocks := int(numBlocks)
	totalBytes := len(compressedData)
	written := 0
	startTime := time.Now()

	for seq := 0; seq < totalBlocks; seq++ {
		start := seq * blockSize
		end := start + blockSize
		if end > len(compressedData) {
			end = len(compressedData)
		}

		block := compressedData[start:end]
		blockData, err := protocol.FlashDeflDataData(block, uint32(seq))
		if err != nil {
			return err
		}
		blockReq := protocol.NewDataRequest(protocol.CmdFlashDeflData, blockData, block)

		// Retry up to 3 times on timeout
		var sendErr error
		for range 3 {
			sendErr = f.sendCommand(blockReq)
			if sendErr == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
			if err := f.port.Flush(); err != nil {
				continue
			}
			f.buf = nil
		}
		if sendErr != nil {
			return fmt.Errorf("flash defl data block %d failed: %w", seq, sendErr)
		}

		written += len(block)
		printProgress(written, totalBytes, startTime)
	}
	fmt.Println() // newline after progress

	// Send FLASH_DEFL_END
	endData := protocol.FlashDeflEndData(false)
	endReq := protocol.NewRequest(protocol.CmdFlashDeflEnd, endData)
	if f.stubRunning {
		// Stub sends a proper response
		if err := f.sendCommand(endReq); err != nil {
			return fmt.Errorf("flash defl end failed: %w", err)
		}
	} else {
		// ROM: fire-and-forget
		frame, err := encodeFrame(endReq)
		if err != nil {
			return err
		}
		if _, err := f.port.Write(frame); err != nil {
			return fmt.Errorf("flash defl end write failed: %w", err)
		}
		if _, err := f.readResponse(protocol.CmdFlashDeflEnd, 2*time.Second); err != nil {
			// The ROM may not reply before it runs the written image.
		}
	}

	return nil
}

// printProgress prints a progress bar with speed info.
func printProgress(written, total int, startTime time.Time) {
	pct := float64(written) / float64(total) * 100
	elapsed := time.Since(startTime).Seconds()
	speed := float64(0)
	if elapsed > 0 {
		speed = float64(written) / 1024 / elapsed
	}

	barWidth := 30
	filled := int(float64(barWidth) * float64(written) / float64(total))
	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "="
		} else if i == filled {
			bar += ">"
		} else {
			bar += " "
		}
	}

	fmt.Printf("\r  [%s] %3.0f%% (%d/%d KB) %.0f KB/s", bar, pct, written/1024, total/1024, speed)
}

// Reboot reboots the device.
func (f *Flasher) Reboot() error {
	endData := protocol.FlashEndData(true)
	endReq := protocol.NewRequest(protocol.CmdFlashEnd, endData)
	frame, err := encodeFrame(endReq)
	if err != nil {
		return err
	}

	_, err = f.port.Write(frame)
	if err != nil {
		return err
	}

	time.Sleep(100 * time.Millisecond)
	return f.port.HardReset()
}

// statusBytes returns the response trailer size: ROM replies carry four
// status bytes, stub flasher replies only two.
func (f *Flasher) statusBytes() int {
	if f.stubRunning {
		return protocol.StubStatusBytes
	}
	return protocol.ROMStatusBytes
}

// sendCommand sends a command and waits for successful response.
func (f *Flasher) sendCommand(req *protocol.Request) error {
	return f.sendCommandWithTimeout(req, 5*time.Second)
}

// sendCommandWithTimeout sends a command with a specific timeout.
func (f *Flasher) sendCommandWithTimeout(req *protocol.Request, timeout time.Duration) error {
	frame, err := encodeFrame(req)
	if err != nil {
		return err
	}

	if _, err := f.port.Write(frame); err != nil {
		return err
	}

	resp, err := f.readResponse(req.Command, timeout)
	if err != nil {
		return err
	}

	if !resp.IsSuccess() {
		return fmt.Errorf("command 0x%02X failed: %s", req.Command, resp.ErrorString())
	}

	return nil
}

func encodeFrame(req *protocol.Request) ([]byte, error) {
	packet, err := req.Encode()
	if err != nil {
		return nil, err
	}
	return slip.Encode(packet), nil
}

// readResponse reads and decodes a protocol response from the shared buffer.
// Non-response SLIP packets (< 10 bytes, like OHAI) are consumed from buf
// and saved in pending for later retrieval by waitForOHAI.
func (f *Flasher) readResponse(expectedCommand byte, timeout time.Duration) (*protocol.Response, error) {
	deadline := time.Now().Add(timeout)
	chunk := make([]byte, 256)

	for time.Now().Before(deadline) {
		// Extract all complete frames, consuming them from buf.
		for {
			frame, remaining := slip.ReadFrame(f.buf)
			if frame == nil {
				break
			}
			f.buf = remaining
			data := slip.Decode(frame)
			if len(data) >= 10 {
				resp, err := protocol.DecodeResponse(data, f.statusBytes())
				if err != nil {
					return nil, err
				}
				if resp.Command != expectedCommand {
					continue
				}
				return resp, nil
			}
			// Non-response packet (e.g. OHAI): save for waitForOHAI.
			f.pending = append(f.pending, data)
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		readTimeout := 100 * time.Millisecond
		if remaining < readTimeout {
			readTimeout = remaining
		}
		n, _ := f.port.ReadWithTimeout(chunk, readTimeout)
		if n > 0 {
			f.buf = append(f.buf, chunk[:n]...)
		}
	}

	return nil, fmt.Errorf("timeout waiting for response")
}
