package detect

import (
	"fmt"

	"github.com/bigbag/papyrix-flasher/internal/flasher"
	"github.com/bigbag/papyrix-flasher/internal/protocol"
	"github.com/bigbag/papyrix-flasher/internal/serial"
)

// Result represents a detected ESP32 device.
type Result struct {
	Port     string
	ChipID   uint32
	ChipName string
}

// DetectDevice returns the first device matching expectedChipID.
func DetectDevice(baudRate int, expectedChipID uint32) (*Result, error) {
	ports, err := serial.ListPorts()
	if err != nil {
		return nil, fmt.Errorf("failed to list ports: %w", err)
	}
	if len(ports) == 0 {
		return nil, fmt.Errorf("no serial ports found")
	}

	var lastErr error
	for _, portName := range ports {
		result, err := tryPort(portName, baudRate)
		if err != nil {
			lastErr = err
			continue
		}
		if expectedChipID == 0 || result.ChipID == expectedChipID {
			return result, nil
		}
		lastErr = fmt.Errorf("%s on %s does not match %s firmware",
			result.ChipName, portName, protocol.ChipName(expectedChipID))
	}

	if lastErr != nil {
		return nil, fmt.Errorf("no %s device found (last error: %w)", protocol.ChipName(expectedChipID), lastErr)
	}
	return nil, fmt.Errorf("no %s device found", protocol.ChipName(expectedChipID))
}

// DetectOnPort identifies an ESP32 device on portName.
func DetectOnPort(portName string, baudRate int) (*Result, error) {
	return tryPort(portName, baudRate)
}

// ListDevices scans all ports and returns devices whose ROM identity is readable.
func ListDevices(baudRate int) ([]Result, error) {
	ports, err := serial.ListPorts()
	if err != nil {
		return nil, fmt.Errorf("failed to list ports: %w", err)
	}

	var results []Result
	for _, portName := range ports {
		result, err := tryPort(portName, baudRate)
		if err == nil {
			results = append(results, *result)
		}
	}
	return results, nil
}

func tryPort(portName string, baudRate int) (*Result, error) {
	port, err := serial.Open(portName, baudRate)
	if err != nil {
		return nil, err
	}
	defer port.Close()

	info, err := flasher.New(port).Identify()
	if err != nil {
		return nil, err
	}
	return &Result{
		Port:     portName,
		ChipID:   info.ChipID,
		ChipName: protocol.ChipName(info.ChipID),
	}, nil
}
