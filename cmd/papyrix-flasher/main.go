package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bigbag/papyrix-flasher/embedded"
	"github.com/bigbag/papyrix-flasher/internal/detect"
	"github.com/bigbag/papyrix-flasher/internal/flasher"
	"github.com/bigbag/papyrix-flasher/internal/protocol"
	"github.com/bigbag/papyrix-flasher/internal/serial"
)

var (
	portFlag         string
	baudFlag         int
	firmwareOnlyFlag bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "papyrix-flasher",
		Short: "Flash firmware to Xteink X4 (ESP32-C3) devices",
	}

	// Flash command
	flashCmd := &cobra.Command{
		Use:   "flash <firmware.bin>",
		Short: "Flash firmware to device",
		Args:  cobra.ExactArgs(1),
		RunE:  runFlash,
	}
	flashCmd.Flags().StringVarP(&portFlag, "port", "p", "", "Serial port (auto-detect if not specified)")
	flashCmd.Flags().IntVarP(&baudFlag, "baud", "b", protocol.DefaultBaudRate, "Baud rate")
	flashCmd.Flags().BoolVar(&firmwareOnlyFlag, "firmware-only", false, "Flash firmware only (skip bootloader/partitions)")

	// Info command
	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Show device info",
		RunE:  runInfo,
	}
	infoCmd.Flags().StringVarP(&portFlag, "port", "p", "", "Serial port (auto-detect if not specified)")
	infoCmd.Flags().IntVarP(&baudFlag, "baud", "b", protocol.DefaultBaudRate, "Baud rate")

	rootCmd.AddCommand(flashCmd, infoCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runFlash(cmd *cobra.Command, args []string) error {
	firmwarePath := args[0]

	// Read firmware file
	firmware, err := os.ReadFile(firmwarePath)
	if err != nil {
		return fmt.Errorf("failed to read firmware file: %w", err)
	}

	fmt.Printf("Firmware: %s (%d bytes)\n", firmwarePath, len(firmware))

	// Find or use specified port
	portName := portFlag
	if portName == "" {
		fmt.Println("Detecting device...")
		result, err := detect.DetectDevice(baudFlag)
		if err != nil {
			return fmt.Errorf("device detection failed: %w", err)
		}
		portName = result.Port
		fmt.Printf("Found %s on %s\n", result.ChipName, result.Port)
	}

	// Open port
	port, err := serial.Open(portName, baudFlag)
	if err != nil {
		return fmt.Errorf("failed to open port: %w", err)
	}
	defer port.Close()

	fmt.Printf("Port: %s @ %d baud\n", portName, baudFlag)

	// Create flasher
	f := flasher.New(port)

	// Connect to bootloader
	fmt.Println("Connecting to bootloader...")
	if err := f.Connect(); err != nil {
		return err
	}
	fmt.Println("Connected!")

	// Prepare regions to flash
	var regions []flasher.FlashRegion

	if !firmwareOnlyFlag {
		regions = append(regions,
			flasher.FlashRegion{
				Address: protocol.BootloaderAddress,
				Data:    embedded.Bootloader(),
				Name:    "bootloader",
			},
			flasher.FlashRegion{
				Address: protocol.PartitionsAddress,
				Data:    embedded.Partitions(),
				Name:    "partitions",
			},
		)
	}

	regions = append(regions, flasher.FlashRegion{
		Address: protocol.FirmwareAddress,
		Data:    firmware,
		Name:    "firmware",
	})

	// Flash each region using compressed transfer
	for _, region := range regions {
		fmt.Printf("Flashing %s at 0x%X (%d bytes)...\n", region.Name, region.Address, len(region.Data))
		if err := f.FlashImageCompressed(region.Data, region.Address, false); err != nil {
			return err
		}
	}

	fmt.Println("\nFlash complete!")

	// Reboot
	fmt.Println("Rebooting device...")
	if err := f.Reboot(); err != nil {
		fmt.Printf("Warning: reboot failed: %v\n", err)
	}

	fmt.Println("Done!")
	fmt.Println("\nNote: To start your device, hold the power button and press the reset button.")
	return nil
}

func runInfo(cmd *cobra.Command, args []string) error {
	if portFlag != "" {
		// Check specific port
		result, err := detect.DetectOnPort(portFlag, baudFlag)
		if err != nil {
			return fmt.Errorf("failed to detect device on %s: %w", portFlag, err)
		}
		printDeviceInfo(result)
		return nil
	}

	// Auto-detect
	fmt.Println("Scanning for ESP32 devices...")
	devices, err := detect.ListDevices(baudFlag)
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		fmt.Println("No ESP32 devices found")
		return nil
	}

	fmt.Printf("Found %d device(s):\n\n", len(devices))
	for i, d := range devices {
		fmt.Printf("Device %d:\n", i+1)
		printDeviceInfo(&d)
		fmt.Println()
	}

	return nil
}

func printDeviceInfo(d *detect.Result) {
	fmt.Printf("  Port:     %s\n", d.Port)
	fmt.Printf("  Chip:     %s\n", d.ChipName)
	if d.ChipID != 0 {
		fmt.Printf("  Chip ID:  0x%02X\n", d.ChipID)
	}
}
