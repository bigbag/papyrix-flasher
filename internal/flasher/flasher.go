package flasher

import (
	"bytes"
	"compress/zlib"
	"fmt"
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

	// Disable watchdogs on USB-JTAG/Serial (ESP32-C3 specific).
	// When using USB-JTAG/Serial, the RTC WDT and SWD are not auto-reset
	// and will reset the board during flashing if not disabled.
	if err := f.disableWatchdogs(); err != nil {
		return fmt.Errorf("failed to disable watchdogs: %w", err)
	}

	// Upload and run stub flasher
	if err := f.uploadStub(); err != nil {
		return fmt.Errorf("failed to upload stub flasher: %w", err)
	}

	return nil
}

// sync sends the SYNC command to establish communication.
func (f *Flasher) sync() error {
	syncReq := protocol.NewRequest(protocol.CmdSync, protocol.SyncData())
	frame := slip.Encode(syncReq.Encode())

	for attempt := 0; attempt < 10; attempt++ {
		f.port.Flush()
		f.buf = nil

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

// readReg reads a 32-bit register on the target.
func (f *Flasher) readReg(addr uint32) (uint32, error) {
	req := protocol.NewRequest(protocol.CmdReadReg, protocol.ReadRegData(addr))
	frame := slip.Encode(req.Encode())

	if _, err := f.port.Write(frame); err != nil {
		return 0, err
	}

	resp, err := f.readResponse(5 * time.Second)
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

// disableWatchdogs disables RTC WDT and auto-feeds SWD when USB-JTAG/Serial is used.
// Matches esptool's ESP32C3ROM._post_connect() -> disable_watchdogs().
func (f *Flasher) disableWatchdogs() error {
	uartNo, err := f.readReg(protocol.UartdevBufNo)
	if err != nil {
		// Can't detect port type — skip watchdog disable (might be UART)
		return nil
	}

	if (uartNo & 0xFF) != protocol.UartdevBufNoUSBJTAG {
		return nil // Not USB-JTAG/Serial, no action needed
	}

	// Disable RTC WDT
	if err := f.writeReg(protocol.RTCCntlWdtWprotectReg, protocol.RTCCntlWdtWkey, 0xFFFFFFFF, 0); err != nil {
		return err
	}
	if err := f.writeReg(protocol.RTCCntlWdtConfig0Reg, 0, 0xFFFFFFFF, 0); err != nil {
		return err
	}
	if err := f.writeReg(protocol.RTCCntlWdtWprotectReg, 0, 0xFFFFFFFF, 0); err != nil {
		return err
	}

	// Auto-feed SWD
	if err := f.writeReg(protocol.RTCCntlSwdWprotectReg, protocol.RTCCntlSwdWkey, 0xFFFFFFFF, 0); err != nil {
		return err
	}
	swdConf, err := f.readReg(protocol.RTCCntlSwdConfReg)
	if err != nil {
		return err
	}
	if err := f.writeReg(protocol.RTCCntlSwdConfReg, swdConf|protocol.RTCCntlSwdAutoFeedEn, 0xFFFFFFFF, 0); err != nil {
		return err
	}
	if err := f.writeReg(protocol.RTCCntlSwdWprotectReg, 0, 0xFFFFFFFF, 0); err != nil {
		return err
	}

	return nil
}

// uploadStub uploads the stub flasher to RAM and executes it.
func (f *Flasher) uploadStub() error {
	s, err := stub.Get()
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
	frame := slip.Encode(endReq.Encode())
	if _, err := f.port.Write(frame); err != nil {
		return fmt.Errorf("mem end write failed: %w", err)
	}
	f.readResponse(200 * time.Millisecond) // best-effort ROM response, ignore error

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

	beginData := protocol.MemBeginData(uint32(len(data)), uint32(numBlocks), uint32(blockSize), offset)
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
		blockData := protocol.MemDataData(block, uint32(seq))
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
func (f *Flasher) FlashImageCompressed(data []byte, address uint32, verify bool) error {
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
	numBlocks := protocol.CalculateDeflBlocks(len(compressedData), blockSize)

	// The stub expects uncompressed size as the first param and erases as it writes.
	// The ROM bootloader expects the erase size rounded to sector boundary.
	eraseSize := uint32(len(data))
	if !f.stubRunning {
		eraseSize = protocol.CalculateEraseSize(len(data))
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
		blockData := protocol.FlashDeflDataData(block, uint32(seq))
		blockReq := protocol.NewDataRequest(protocol.CmdFlashDeflData, blockData, block)

		// Retry up to 3 times on timeout
		var sendErr error
		for attempt := 0; attempt < 3; attempt++ {
			sendErr = f.sendCommand(blockReq)
			if sendErr == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
			f.port.Flush()
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
		frame := slip.Encode(endReq.Encode())
		f.port.Write(frame)
		f.readResponse(2 * time.Second)
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

// readResponse reads and decodes a protocol response from the shared buffer.
// Non-response SLIP packets (< 10 bytes, like OHAI) are consumed from buf
// and saved in pending for later retrieval by waitForOHAI.
func (f *Flasher) readResponse(timeout time.Duration) (*protocol.Response, error) {
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
				return protocol.DecodeResponse(data)
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
