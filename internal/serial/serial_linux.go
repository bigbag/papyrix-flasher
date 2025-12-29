//go:build linux

package serial

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// termios constants for Linux
const (
	TCGETS   = 0x5401
	TCSETS   = 0x5402
	TCSETSW  = 0x5403
	TCSBRK   = 0x5409
	TCFLSH   = 0x540B
	TIOCSBRK = 0x5427
	TIOCCBRK = 0x5428

	// c_iflag
	IGNBRK = 0x1
	BRKINT = 0x2
	IGNPAR = 0x4
	PARMRK = 0x8
	INPCK  = 0x10
	ISTRIP = 0x20
	INLCR  = 0x40
	IGNCR  = 0x80
	ICRNL  = 0x100
	IXON   = 0x400
	IXANY  = 0x800
	IXOFF  = 0x1000

	// c_oflag
	OPOST = 0x1

	// c_cflag
	CSIZE   = 0x30
	CS5     = 0x0
	CS6     = 0x10
	CS7     = 0x20
	CS8     = 0x30
	CSTOPB  = 0x40
	CREAD   = 0x80
	PARENB  = 0x100
	PARODD  = 0x200
	HUPCL   = 0x400
	CLOCAL  = 0x800
	CRTSCTS = 0x80000000

	// c_lflag
	ISIG   = 0x1
	ICANON = 0x2
	ECHO   = 0x8
	ECHOE  = 0x10
	ECHOK  = 0x20
	ECHONL = 0x40
	IEXTEN = 0x8000

	// VMIN/VTIME indices
	VMIN  = 6
	VTIME = 5

	// tcflush constants
	TCIFLUSH  = 0
	TCOFLUSH  = 1
	TCIOFLUSH = 2

	// DTR/RTS control
	TIOCM_DTR = 0x002
	TIOCM_RTS = 0x004
	TIOCMGET  = 0x5415
	TIOCMSET  = 0x5418
	TIOCMBIS  = 0x5416
	TIOCMBIC  = 0x5417
)

// Baud rate constants
var baudRates = map[int]uint32{
	9600:   0xd,
	19200:  0xe,
	38400:  0xf,
	57600:  0x1001,
	115200: 0x1002,
	230400: 0x1003,
	460800: 0x1004,
	500000: 0x1005,
	576000: 0x1006,
	921600: 0x1007,
}

// termios structure for Linux
type termios struct {
	Iflag  uint32
	Oflag  uint32
	Cflag  uint32
	Lflag  uint32
	Line   uint8
	Cc     [32]uint8
	Ispeed uint32
	Ospeed uint32
}

// RawPort is a serial port using raw syscalls
type RawPort struct {
	fd       int
	file     *os.File
	portName string
	baudRate int
}

// OpenRaw opens a serial port using raw syscalls
func OpenRaw(portName string, baudRate int) (*RawPort, error) {
	// Open with O_RDWR | O_NOCTTY | O_NONBLOCK
	fd, err := syscall.Open(portName, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open port %s: %w", portName, err)
	}

	// Clear non-blocking mode after open using Syscall
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFL, 0)
	if errno == 0 {
		syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_SETFL, flags&^syscall.O_NONBLOCK)
	}

	port := &RawPort{
		fd:       fd,
		file:     os.NewFile(uintptr(fd), portName),
		portName: portName,
		baudRate: baudRate,
	}

	// Configure the port
	if err := port.configure(); err != nil {
		syscall.Close(fd)
		return nil, err
	}

	return port, nil
}

func (p *RawPort) configure() error {
	var t termios

	// Get current termios
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(p.fd), TCGETS, uintptr(unsafe.Pointer(&t))); errno != 0 {
		return fmt.Errorf("tcgetattr failed: %v", errno)
	}

	// Get baud rate code
	baudCode, ok := baudRates[p.baudRate]
	if !ok {
		return fmt.Errorf("unsupported baud rate: %d", p.baudRate)
	}

	// Configure for raw mode (like cfmakeraw)
	t.Iflag &^= IGNBRK | BRKINT | PARMRK | ISTRIP | INLCR | IGNCR | ICRNL | IXON | IXOFF | IXANY
	t.Oflag &^= OPOST
	t.Lflag &^= ECHO | ECHONL | ICANON | ISIG | IEXTEN
	t.Cflag &^= CSIZE | PARENB | PARODD | CSTOPB | CRTSCTS

	// 8N1, enable receiver, local mode
	t.Cflag |= CS8 | CREAD | CLOCAL

	// Set baud rate
	t.Ispeed = baudCode
	t.Ospeed = baudCode

	// VMIN=0, VTIME=1 (100ms timeout for reads)
	t.Cc[VMIN] = 0
	t.Cc[VTIME] = 1

	// Set termios
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(p.fd), TCSETSW, uintptr(unsafe.Pointer(&t))); errno != 0 {
		return fmt.Errorf("tcsetattr failed: %v", errno)
	}

	return nil
}

// Close closes the serial port
func (p *RawPort) Close() error {
	if p.file != nil {
		return p.file.Close()
	}
	return nil
}

// Write writes data to the serial port
func (p *RawPort) Write(data []byte) (int, error) {
	// Simple write like pyserial does
	n, err := syscall.Write(p.fd, data)
	if err != nil {
		return n, err
	}
	// Drain after write to ensure data is transmitted
	p.drain()
	return n, nil
}

// drain waits for output to be transmitted (tcdrain)
func (p *RawPort) drain() error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(p.fd), TCSBRK, 1)
	if errno != 0 {
		return errno
	}
	return nil
}

// Read reads data from the serial port
func (p *RawPort) Read(buf []byte) (int, error) {
	return syscall.Read(p.fd, buf)
}

// ReadWithTimeout reads data with a specific timeout
func (p *RawPort) ReadWithTimeout(buf []byte, timeout time.Duration) (int, error) {
	// Set VTIME based on timeout (in 0.1 second units)
	vtime := int(timeout.Milliseconds() / 100)
	if vtime < 1 {
		vtime = 1
	}
	if vtime > 255 {
		vtime = 255
	}

	var t termios
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(p.fd), TCGETS, uintptr(unsafe.Pointer(&t))); errno != 0 {
		return 0, errno
	}

	oldVtime := t.Cc[VTIME]
	t.Cc[VTIME] = uint8(vtime)

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(p.fd), TCSETSW, uintptr(unsafe.Pointer(&t))); errno != 0 {
		return 0, errno
	}

	n, err := syscall.Read(p.fd, buf)

	// Restore VTIME
	t.Cc[VTIME] = oldVtime
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(p.fd), TCSETSW, uintptr(unsafe.Pointer(&t)))

	return n, err
}

// Flush discards any buffered data
func (p *RawPort) Flush() error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(p.fd), TCFLSH, TCIOFLUSH)
	if errno != 0 {
		return errno
	}
	return nil
}

// SetDTR sets the DTR signal
func (p *RawPort) SetDTR(value bool) error {
	var bits int
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(p.fd), TIOCMGET, uintptr(unsafe.Pointer(&bits))); errno != 0 {
		return errno
	}

	if value {
		bits |= TIOCM_DTR
	} else {
		bits &^= TIOCM_DTR
	}

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(p.fd), TIOCMSET, uintptr(unsafe.Pointer(&bits))); errno != 0 {
		return errno
	}
	return nil
}

// SetRTS sets the RTS signal
func (p *RawPort) SetRTS(value bool) error {
	var bits int
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(p.fd), TIOCMGET, uintptr(unsafe.Pointer(&bits))); errno != 0 {
		return errno
	}

	if value {
		bits |= TIOCM_RTS
	} else {
		bits &^= TIOCM_RTS
	}

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(p.fd), TIOCMSET, uintptr(unsafe.Pointer(&bits))); errno != 0 {
		return errno
	}
	return nil
}

// ResetToBootloader resets the ESP32 into bootloader mode
func (p *RawPort) ResetToBootloader() error {
	// Classic reset sequence for ESP32 bootloader
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

// HardReset performs a hard reset
func (p *RawPort) HardReset() error {
	if err := p.SetRTS(true); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	if err := p.SetRTS(false); err != nil {
		return err
	}
	return nil
}

// PortName returns the port name
func (p *RawPort) PortName() string {
	return p.portName
}

// BaudRate returns the current baud rate
func (p *RawPort) BaudRate() int {
	return p.baudRate
}
