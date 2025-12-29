package main

import (
	"fmt"
	"os"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"github.com/bigbag/papyrix-flasher/embedded"
	"github.com/bigbag/papyrix-flasher/internal/detect"
	"github.com/bigbag/papyrix-flasher/internal/flasher"
	"github.com/bigbag/papyrix-flasher/internal/protocol"
	"github.com/bigbag/papyrix-flasher/internal/serial"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	portFlag         string
	baudFlag         int
	verifyFlag       bool
	firmwareOnlyFlag bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "papyrix-flasher",
		Short: "Flash firmware to Xteink X4 (ESP32-C3) devices",
		Long: `Papyrix Flasher is a cross-platform tool for flashing Papyrix firmware
to Xteink X4 e-paper reader devices powered by ESP32-C3.

The bootloader and partition table are embedded in this tool.
You only need to provide the firmware.bin file.`,
	}

	// Flash command
	flashCmd := &cobra.Command{
		Use:   "flash <firmware.bin>",
		Short: "Flash firmware to device",
		Long: `Flash firmware to an ESP32-C3 device.

By default, this will flash:
  - Bootloader at 0x0000 (embedded)
  - Partition table at 0x8000 (embedded)
  - Firmware at 0x10000 (your file)

Use --firmware-only to skip bootloader and partition table.`,
		Args: cobra.ExactArgs(1),
		RunE: runFlash,
	}
	flashCmd.Flags().StringVarP(&portFlag, "port", "p", "", "Serial port (auto-detect if not specified)")
	flashCmd.Flags().IntVarP(&baudFlag, "baud", "b", protocol.DefaultBaudRate, "Baud rate")
	flashCmd.Flags().BoolVar(&verifyFlag, "verify", true, "Verify after flashing")
	flashCmd.Flags().BoolVar(&firmwareOnlyFlag, "firmware-only", false, "Flash firmware only (skip bootloader/partitions)")

	// Info command
	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Show device info",
		Long:  "Detect and show information about connected ESP32 devices.",
		RunE:  runInfo,
	}
	infoCmd.Flags().StringVarP(&portFlag, "port", "p", "", "Serial port (auto-detect if not specified)")
	infoCmd.Flags().IntVarP(&baudFlag, "baud", "b", protocol.DefaultBaudRate, "Baud rate")

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version info",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("papyrix-flasher %s\n", version)
			fmt.Printf("  commit: %s\n", commit)
			fmt.Printf("  built:  %s\n", date)
		},
	}

	// List command
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available serial ports",
		RunE:  runList,
	}

	rootCmd.AddCommand(flashCmd, infoCmd, versionCmd, listCmd)

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

	// Calculate total size
	totalSize := 0
	for _, r := range regions {
		totalSize += len(r.Data)
	}
	totalBlocks := totalSize / protocol.FlashBlockSize
	if totalSize%protocol.FlashBlockSize != 0 {
		totalBlocks++
	}

	// Flash each region
	currentBlock := 0
	bar := progressbar.NewOptions(totalBlocks,
		progressbar.OptionSetDescription("Flashing"),
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowBytes(false),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionThrottle(100),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
	)

	for _, region := range regions {
		regionBlocks := len(region.Data) / protocol.FlashBlockSize
		if len(region.Data)%protocol.FlashBlockSize != 0 {
			regionBlocks++
		}

		f.SetProgressCallback(func(current, total int) {
			bar.Set(currentBlock + current)
		})

		fmt.Printf("\nFlashing %s at 0x%X (%d bytes)...\n", region.Name, region.Address, len(region.Data))
		if err := f.FlashImage(region.Data, region.Address, verifyFlag); err != nil {
			return err
		}

		currentBlock += regionBlocks
	}

	bar.Finish()
	fmt.Println("\nFlash complete!")

	// Reboot
	fmt.Println("Rebooting device...")
	if err := f.Reboot(); err != nil {
		fmt.Printf("Warning: reboot failed: %v\n", err)
	}

	fmt.Println("Done!")
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

func runList(cmd *cobra.Command, args []string) error {
	ports, err := serial.ListPorts()
	if err != nil {
		return err
	}

	if len(ports) == 0 {
		fmt.Println("No serial ports found")
		return nil
	}

	fmt.Println("Available serial ports:")
	for _, p := range ports {
		fmt.Printf("  %s\n", p)
	}

	return nil
}
