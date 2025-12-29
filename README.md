# Papyrix Flasher

A cross-platform command-line tool for flashing [Papyrix](https://github.com/bigbag/papyrix-reader) firmware to Xteink X4 e-paper reader devices (ESP32-C3).

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

| Region | Address | Size | Description |
|--------|---------|------|-------------|
| Bootloader | 0x0000 | ~12KB | ESP32-C3 second-stage bootloader |
| Partitions | 0x8000 | 3KB | Partition table |
| App (OTA 0) | 0x10000 | ~6.3MB | Main application |
| App (OTA 1) | 0x650000 | ~6.3MB | OTA update partition |
| SPIFFS | 0xC90000 | ~3.3MB | File storage |

By default, `papyrix-flasher` writes to:
- Bootloader at 0x0000 (embedded in tool)
- Partition table at 0x8000 (embedded in tool)
- Firmware at 0x10000 (your file)

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
│   ├── protocol/           # ESP32 bootloader protocol
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

# Run tests
make test

# Update embedded binaries from papyrix-reader
make update-embedded
```

## References

- [Espressif esptool documentation](https://docs.espressif.com/projects/esptool/en/latest/)

## License

MIT License - see [LICENSE](LICENSE) for details.
