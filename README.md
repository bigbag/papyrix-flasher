# Papyrix Flasher

[![Build](https://github.com/bigbag/papyrix-flasher/workflows/Build/badge.svg)](https://github.com/bigbag/papyrix-flasher/actions?query=workflow%3ABuild)
[![Go](https://img.shields.io/badge/go-1.22-blue.svg)](https://github.com/bigbag/papyrix-flasher)
[![license](https://img.shields.io/github/license/bigbag/papyrix-flasher.svg)](https://github.com/bigbag/papyrix-flasher/blob/main/LICENSE)

A cross-platform command-line tool for flashing firmware to Xteink X4 e-paper reader devices (ESP32-C3). Works with any ESP32-C3 compatible firmware, including [Papyrix](https://github.com/bigbag/papyrix-reader) and [CrossPoint Reader](https://github.com/daveallie/crosspoint-reader).

## Features

- **Simple**: Just run `papyrix-flasher flash firmware.bin` - bootloader and partition table are embedded
- **Auto-detect**: Automatically finds connected ESP32-C3 devices
- **Cross-platform**: Works on Windows, Linux, and macOS
- **Verification**: MD5 verification after flashing (enabled by default)
- **Progress bar**: Visual progress during flashing

## Installation

### Pre-built binaries

Download the latest release for your platform from the [Releases](https://github.com/bigbag/papyrix-flasher/releases) page.

### Build from source

```bash
git clone https://github.com/bigbag/papyrix-flasher.git
cd papyrix-flasher
make build
```

## Usage

### Flash firmware

```bash
# Auto-detect device and flash
papyrix-flasher flash firmware.bin

# Specify port explicitly
papyrix-flasher flash -p /dev/ttyUSB0 firmware.bin    # Linux
papyrix-flasher flash -p /dev/cu.usbserial-* firmware.bin  # macOS
papyrix-flasher flash -p COM3 firmware.bin            # Windows

# Flash firmware only (skip bootloader/partitions for faster updates)
papyrix-flasher flash --firmware-only firmware.bin

# Skip verification (faster, but risky)
papyrix-flasher flash --verify=false firmware.bin
```

### Show device info

```bash
# Auto-detect and show device info
papyrix-flasher info

# Check specific port
papyrix-flasher info -p /dev/ttyUSB0
```

### List serial ports

```bash
papyrix-flasher list
```

### Version info

```bash
papyrix-flasher version
```

## Flash Memory Layout

The Xteink X4 has 16MB of flash memory, organized as:

- **Bootloader** at `0x0000` (~12KB) - ESP32-C3 second-stage bootloader
- **Partitions** at `0x8000` (3KB) - Partition table
- **App (OTA 0)** at `0x10000` (~6.3MB) - Main application
- **App (OTA 1)** at `0x650000` (~6.3MB) - OTA update partition
- **SPIFFS** at `0xC90000` (~3.3MB) - File storage

By default, `papyrix-flasher` writes to:
- Bootloader at 0x0000 (embedded in tool)
- Partition table at 0x8000 (embedded in tool)
- Firmware at 0x10000 (your file)

## How It Works

This tool implements the ESP32 ROM bootloader protocol to flash firmware over a USB serial connection.

### Communication Stack

- **Serial**: 921600 baud, 8N1, no flow control
- **Framing**: SLIP (Serial Line Internet Protocol) with `0xC0` delimiters and escape sequences for special bytes
- **Protocol**: ESP32 ROM bootloader binary protocol with request/response packets and XOR checksum

### Bootloader Entry

The ESP32-C3 enters bootloader mode via DTR/RTS signal sequence that controls the EN (reset) and GPIO0 (boot mode) pins through transistor drivers:

1. Assert EN low (reset the chip)
2. Assert GPIO0 low while releasing EN (boot into download mode)
3. Release GPIO0 (chip stays in bootloader)

### Flash Sequence

1. **Reset to bootloader** - DTR/RTS signal sequence
2. **SYNC** - Establish communication with bootloader (up to 10 retries)
3. **SPI_ATTACH** - Attach the SPI flash chip
4. **SPI_SET_PARAMS** - Configure flash size (16MB)
5. **FLASH_DEFL_BEGIN** - Start compressed flash session, erase sectors
6. **FLASH_DEFL_DATA** - Send zlib-compressed firmware in 1KB blocks (with retry on failure)
7. **FLASH_DEFL_END** - Finalize flash session
8. **Hard reset** - Reboot into the new firmware

### Compression

Firmware is compressed using zlib (deflate) before transfer. The bootloader decompresses data on-the-fly, reducing transfer time significantly (typically 2-4x compression ratio).

## Troubleshooting

### Device not detected

1. Ensure the device is connected via USB
2. Check if the device appears in `papyrix-flasher list`
3. On Linux, you may need to add yourself to the `dialout` group:
   ```bash
   sudo usermod -a -G dialout $USER
   ```
   Then log out and back in.

### Permission denied

On Linux/macOS, you may need to run with sudo or fix port permissions:
```bash
sudo papyrix-flasher flash firmware.bin
# Or fix permissions permanently (Linux)
sudo chmod 666 /dev/ttyUSB0
```

### Sync failed

1. Try pressing the reset button on the device while the tool is connecting
2. Make sure no other program is using the serial port
3. Try a different USB cable

## Development

### Project structure

```
papyrix-flasher/
├── cmd/papyrix-flasher/    # CLI entry point
├── internal/
│   ├── slip/               # SLIP protocol encoding/decoding
│   │   ├── slip.go
│   │   └── slip_test.go
│   ├── protocol/           # ESP32 bootloader protocol
│   │   ├── commands.go
│   │   ├── commands_test.go
│   │   ├── packet.go
│   │   ├── packet_test.go
│   │   └── esp32c3.go
│   ├── serial/             # Serial port abstraction
│   ├── detect/             # Device auto-detection
│   └── flasher/            # High-level flash operations
├── embedded/               # Embedded bootloader and partitions
├── Makefile
└── README.md
```

### Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Update embedded binaries from papyrix-reader
make update-embedded

# Create and push a release tag (triggers GitHub release workflow)
make tag
```

### Testing

```bash
# Run all tests
make test

# Run tests with verbose output
go test -v ./...

# Run tests for specific packages
go test -v ./internal/slip ./internal/protocol
```

Unit tests cover the core protocol packages:
- **slip**: SLIP framing encode/decode, escape sequences, frame extraction
- **protocol**: Packet encoding/decoding, checksum calculation, command data generation

## References

- [Espressif esptool documentation](https://docs.espressif.com/projects/esptool/en/latest/)

## License

MIT License - see [LICENSE](LICENSE) for details.
