//go:build !linux

package serial

import (
	"errors"
	"time"
)

// RawPort is a stub for non-Linux platforms.
// This is never used at runtime (see Open function in serial.go).
type RawPort struct{}

// OpenRaw is a stub for non-Linux platforms.
// This is never called at runtime.
func OpenRaw(portName string, baudRate int) (*RawPort, error) {
	return nil, errors.New("raw serial port not supported on this platform")
}

// Close is a stub - never called on non-Linux platforms.
func (p *RawPort) Close() error {
	return errors.New("raw serial port not supported on this platform")
}

// Write is a stub - never called on non-Linux platforms.
func (p *RawPort) Write(data []byte) (int, error) {
	return 0, errors.New("raw serial port not supported on this platform")
}

// Read is a stub - never called on non-Linux platforms.
func (p *RawPort) Read(buf []byte) (int, error) {
	return 0, errors.New("raw serial port not supported on this platform")
}

// ReadWithTimeout is a stub - never called on non-Linux platforms.
func (p *RawPort) ReadWithTimeout(buf []byte, timeout time.Duration) (int, error) {
	return 0, errors.New("raw serial port not supported on this platform")
}

// Flush is a stub - never called on non-Linux platforms.
func (p *RawPort) Flush() error {
	return errors.New("raw serial port not supported on this platform")
}

// SetDTR is a stub - never called on non-Linux platforms.
func (p *RawPort) SetDTR(value bool) error {
	return errors.New("raw serial port not supported on this platform")
}

// SetRTS is a stub - never called on non-Linux platforms.
func (p *RawPort) SetRTS(value bool) error {
	return errors.New("raw serial port not supported on this platform")
}

// ResetToBootloader is a stub - never called on non-Linux platforms.
func (p *RawPort) ResetToBootloader() error {
	return errors.New("raw serial port not supported on this platform")
}

// HardReset is a stub - never called on non-Linux platforms.
func (p *RawPort) HardReset() error {
	return errors.New("raw serial port not supported on this platform")
}
