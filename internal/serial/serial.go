package serial

import (
	"fmt"
	"time"

	"go.bug.st/serial"
)

// Port wraps a serial port with ESP32-specific functionality.
type Port struct {
	port     serial.Port
	portName string
	baudRate int
}

// Open opens a serial port with the specified baud rate.
func Open(portName string, baudRate int) (*Port, error) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open port %s: %w", portName, err)
	}

	// Set read timeout
	if err := port.SetReadTimeout(100 * time.Millisecond); err != nil {
		port.Close()
		return nil, fmt.Errorf("failed to set read timeout: %w", err)
	}

	return &Port{
		port:     port,
		portName: portName,
		baudRate: baudRate,
	}, nil
}

// Close closes the serial port.
func (p *Port) Close() error {
	if p.port != nil {
		return p.port.Close()
	}
	return nil
}

// Write writes data to the serial port.
func (p *Port) Write(data []byte) (int, error) {
	return p.port.Write(data)
}

// Read reads data from the serial port.
func (p *Port) Read(buf []byte) (int, error) {
	return p.port.Read(buf)
}

// ReadWithTimeout reads data with a specific timeout.
func (p *Port) ReadWithTimeout(buf []byte, timeout time.Duration) (int, error) {
	if err := p.port.SetReadTimeout(timeout); err != nil {
		return 0, err
	}
	defer p.port.SetReadTimeout(100 * time.Millisecond)

	return p.port.Read(buf)
}

// ReadAll reads all available data with a timeout.
func (p *Port) ReadAll(timeout time.Duration) ([]byte, error) {
	if err := p.port.SetReadTimeout(timeout); err != nil {
		return nil, err
	}

	var result []byte
	buf := make([]byte, 1024)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		n, err := p.port.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err != nil {
			break
		}
		if n == 0 {
			break
		}
	}

	return result, nil
}

// Flush discards any buffered data.
func (p *Port) Flush() error {
	return p.port.ResetInputBuffer()
}

// SetDTR sets the DTR signal.
func (p *Port) SetDTR(value bool) error {
	return p.port.SetDTR(value)
}

// SetRTS sets the RTS signal.
func (p *Port) SetRTS(value bool) error {
	return p.port.SetRTS(value)
}

// ResetToBootloader resets the ESP32 into bootloader mode using DTR/RTS.
// This uses the common auto-reset circuit used on most ESP32 dev boards.
func (p *Port) ResetToBootloader() error {
	// Classic reset sequence:
	// 1. RTS high, DTR low -> EN low (reset), GPIO0 high
	// 2. RTS low, DTR high -> EN high (run), GPIO0 low (boot mode)
	// 3. RTS high, DTR low -> release GPIO0

	// Note: Signal polarities are inverted due to transistor drivers

	// Step 1: Assert EN (reset)
	if err := p.SetRTS(true); err != nil {
		return err
	}
	if err := p.SetDTR(false); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)

	// Step 2: Assert GPIO0 (boot mode), release EN
	if err := p.SetRTS(false); err != nil {
		return err
	}
	if err := p.SetDTR(true); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)

	// Step 3: Release GPIO0
	if err := p.SetRTS(true); err != nil {
		return err
	}
	if err := p.SetDTR(false); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)

	// Final: Release all
	if err := p.SetRTS(false); err != nil {
		return err
	}
	if err := p.SetDTR(false); err != nil {
		return err
	}

	// Flush any garbage from reset
	p.Flush()
	time.Sleep(100 * time.Millisecond)

	return nil
}

// HardReset performs a hard reset (without entering bootloader).
func (p *Port) HardReset() error {
	// Pull EN low then release
	if err := p.SetRTS(true); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	if err := p.SetRTS(false); err != nil {
		return err
	}
	return nil
}

// PortName returns the port name.
func (p *Port) PortName() string {
	return p.portName
}

// BaudRate returns the current baud rate.
func (p *Port) BaudRate() int {
	return p.baudRate
}

// ListPorts returns a list of available serial ports.
func ListPorts() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}
	return ports, nil
}
