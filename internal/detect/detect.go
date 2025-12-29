package detect

import (
	"fmt"
	"time"

	"github.com/bigbag/papyrix-flasher/internal/protocol"
	"github.com/bigbag/papyrix-flasher/internal/serial"
	"github.com/bigbag/papyrix-flasher/internal/slip"
)

// Result represents a detected ESP32 device.
type Result struct {
	Port     string
	ChipID   uint32
	ChipName string
}

// DetectDevice tries to detect an ESP32 device on available ports.
// Returns the first detected ESP32-C3 device, or an error.
func DetectDevice(baudRate int) (*Result, error) {
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
		return result, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("no ESP32-C3 device found (last error: %w)", lastErr)
	}
	return nil, fmt.Errorf("no ESP32-C3 device found")
}

// DetectOnPort tries to detect an ESP32 device on a specific port.
func DetectOnPort(portName string, baudRate int) (*Result, error) {
	return tryPort(portName, baudRate)
}

// ListDevices scans all ports and returns all detected ESP32 devices.
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

	// Try to reset into bootloader
	if err := port.ResetToBootloader(); err != nil {
		return nil, fmt.Errorf("failed to reset: %w", err)
	}

	// Try to sync with the bootloader
	if err := syncWithBootloader(port); err != nil {
		return nil, fmt.Errorf("failed to sync: %w", err)
	}

	// Get chip info
	chipID, err := getChipID(port)
	if err != nil {
		// Even if we can't get chip ID, sync worked so it's likely an ESP32
		return &Result{
			Port:     portName,
			ChipID:   0,
			ChipName: "ESP32 (unknown variant)",
		}, nil
	}

	return &Result{
		Port:     portName,
		ChipID:   chipID,
		ChipName: protocol.ChipName(chipID),
	}, nil
}

func syncWithBootloader(port *serial.Port) error {
	syncReq := protocol.NewRequest(protocol.CmdSync, protocol.SyncData())
	frame := slip.Encode(syncReq.Encode())

	// Try sync multiple times
	for attempt := 0; attempt < 5; attempt++ {
		// Send sync command
		if _, err := port.Write(frame); err != nil {
			continue
		}

		// Wait for response
		time.Sleep(50 * time.Millisecond)

		// Read response
		response, err := port.ReadAll(200 * time.Millisecond)
		if err != nil {
			continue
		}

		if len(response) == 0 {
			continue
		}

		// Try to decode response
		respFrame, _ := slip.ReadFrame(response)
		if respFrame == nil {
			continue
		}

		data := slip.Decode(respFrame)
		if len(data) < 10 {
			continue
		}

		resp, err := protocol.DecodeResponse(data)
		if err != nil {
			continue
		}

		if resp.Command == protocol.CmdSync && resp.IsSuccess() {
			return nil
		}
	}

	return fmt.Errorf("sync failed after 5 attempts")
}

func getChipID(port *serial.Port) (uint32, error) {
	// Send GET_SECURITY_INFO command to get chip info
	req := protocol.NewRequest(protocol.CmdGetSecurityInfo, nil)
	frame := slip.Encode(req.Encode())

	if _, err := port.Write(frame); err != nil {
		return 0, err
	}

	time.Sleep(50 * time.Millisecond)

	response, err := port.ReadAll(200 * time.Millisecond)
	if err != nil {
		return 0, err
	}

	respFrame, _ := slip.ReadFrame(response)
	if respFrame == nil {
		return 0, fmt.Errorf("no response frame")
	}

	data := slip.Decode(respFrame)
	resp, err := protocol.DecodeResponse(data)
	if err != nil {
		return 0, err
	}

	if !resp.IsSuccess() {
		return 0, fmt.Errorf("get security info failed: %s", resp.ErrorString())
	}

	info, err := protocol.ParseSecurityInfo(resp.Data)
	if err != nil {
		return 0, err
	}

	return info.ChipID, nil
}
