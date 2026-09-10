# Papyrix Flasher

[![Build](https://github.com/bigbag/papyrix-flasher/workflows/Build/badge.svg)](https://github.com/bigbag/papyrix-flasher/actions?query=workflow%3ABuild)
[![Go](https://img.shields.io/badge/go-1.25-blue.svg)](https://github.com/bigbag/papyrix-flasher)
[![license](https://img.shields.io/github/license/bigbag/papyrix-flasher.svg)](https://github.com/bigbag/papyrix-flasher/blob/main/LICENSE)

Papyrix Flasher installs Papyrix application images on supported Xteink e-paper readers. It implements the Espressif ROM bootloader protocol and includes the required bootloaders and partition table.

## Supported devices

| Devices | MCU | Papyrix release file |
| --- | --- | --- |
| Xteink X3 and X4 | ESP32-C3 | `papyrix-xteink-c3.bin` |
| Xteink X4 Pro | ESP32-S3 | `papyrix-x4pro.bin` |

The input must be an ESP-IDF application image for the detected MCU. The image must fit the Papyrix `app0` partition. A matching MCU does not prove that firmware supports the board, display, PSRAM, or other hardware.

## Installation

Download a binary for your operating system from the [Releases](https://github.com/bigbag/papyrix-flasher/releases) page.

To build from source, install the Go version from `go.mod`, then run:

```sh
git clone https://github.com/bigbag/papyrix-flasher.git
cd papyrix-flasher
make build
```

## Flash firmware

Auto-detect an X3 or X4 and install the C3 release:

```sh
papyrix-flasher flash papyrix-xteink-c3.bin
```

Install the X4 Pro release on a specific port:

```sh
papyrix-flasher flash -p /dev/ttyACM0 papyrix-x4pro.bin
```

Install only the application:

```sh
papyrix-flasher flash --firmware-only -p /dev/ttyACM0 papyrix-x4pro.bin
```

Show ROM device information:

```sh
papyrix-flasher info
papyrix-flasher info -p /dev/ttyACM0
```

Use `--baud` to change the default rate of 921600. Native USB ignores this value, but a UART adapter uses it.

### X4 Pro power requirement

Use an X4 Pro with unlocked USB flashing. Use a data-capable USB cable. Close every serial monitor before flashing.

Hold **Power** before the flasher starts device detection. Keep Power pressed throughout flashing. Release Power only after the application starts. The ESP32-S3 ROM does not assert the X4 Pro power latch.

The current X4 Pro path supports hardware USB Serial/JTAG. It rejects the USB-OTG ROM path instead of using unsafe transfer limits.

### Full flash and firmware-only

The default command writes:

- the MCU-specific bootloader at `0x0000`;
- the shared partition table at `0x8000`;
- erased OTA state at `0xE000`;
- the application at `0x10000`.

`--firmware-only` writes only the application at `0x10000`. It leaves the bootloader, partition table, and OTA state unchanged.

Do not use full flash for firmware that requires a different partition layout. Full flash replaces the current bootloader and partition table.

## Safety checks

Before it opens a serial port, the flasher checks:

- ESP image magic and segment bounds;
- the ESP-IDF application descriptor;
- application size against the `app0` partition;
- segment XOR checksum;
- the appended SHA-256 digest when present;
- target MCU: ESP32-C3 or ESP32-S3.

After ROM identification, the flasher rejects a firmware/device MCU mismatch. It also rejects secure boot, secure download mode, and flash encryption because this tool does not support these modes.

These checks validate the input file and target choice. They do **not** read flash back from the device. A `Flash complete!` message means that the ROM or RAM stub accepted the write commands. It is not an independent device readback verification.

## Flash memory layout

Both targets use 16 MB flash and this partition layout:

| Address | Size | Content |
| --- | --- | --- |
| `0x0000` | target-specific | Bootloader |
| `0x8000` | `0x1000` | Partition table |
| `0x9000` | `0x5000` | NVS |
| `0xE000` | `0x2000` | OTA state |
| `0x10000` | `0x640000` | Application slot 0 |
| `0x650000` | `0x640000` | Application slot 1 |
| `0xC90000` | `0x360000` | SPIFFS/LittleFS data |
| `0xFF0000` | `0x10000` | Core dump |

## Protocol flow

1. Reset the device into ROM download mode.
2. Send `SYNC` and read `GET_SECURITY_INFO`.
3. Check the chip and security state.
4. Configure the chip-specific USB watchdog registers.
5. Upload and start the chip-specific RAM stub.
6. Send zlib-compressed flash blocks.
7. Reboot the device.

ROM replies use a four-byte status trailer. RAM-stub replies use a two-byte status trailer. The flasher handles both formats explicitly.

## Troubleshooting

### Device not found

1. Close serial monitors and other programs that use the port.
2. Use a data-capable USB cable.
3. Run `papyrix-flasher info -p <port>`.
4. On X4 Pro, hold Power before running the command.
5. On Linux, confirm that your user can access the serial device.

Typical native USB ports are `/dev/ttyACM0` on Linux and `/dev/cu.usbmodem*` on macOS. Windows uses a `COM` port.

### Firmware/device mismatch

Use `papyrix-xteink-c3.bin` for X3 or X4. Use `papyrix-x4pro.bin` for X4 Pro. The flasher stops before register, RAM, or flash writes when the MCU targets differ.

### Device remains in download mode

`info` enters ROM download mode to identify the MCU. Restart the device after the command if the application does not start automatically.

## Development

```sh
make build       # Build the current platform
make build-all   # Build supported release platforms
make test        # Run Go tests
make fmt         # Format Go code
make lint        # Run golangci-lint when installed
```

Build both release targets in `papyrix-reader`, then refresh the embedded assets:

```sh
# In papyrix-reader
make release

# In papyrix-flasher
make update-embedded
make test
make build-all
```

`READER_DIR` defaults to `../papyrix-reader`. Override it when the repositories are not siblings.

`make update-embedded` checks both bootloader chip IDs before it copies files. It also requires the C3 and S3 partition binaries to match.

The S3 RAM stub comes from pioarduino `tool-esptoolpy` 5.1.2 generation 1. The checked-in JSON SHA-256 is `4f0baa7f900215f9942bd7c0f93cd87c4023e6d8ce357c29bf5ae500ccd62ec4`.

## Source layout

```text
cmd/papyrix-flasher/  CLI
embedded/             C3/S3 bootloaders and shared partition table
internal/detect/      Serial-port enumeration
internal/flasher/     ROM identification, stub upload, and flash operations
internal/protocol/    Packet and ESP application-image validation
internal/serial/      Cross-platform serial and reset control
internal/slip/        SLIP framing
internal/stub/        C3/S3 RAM stubs
```

## References

- [Espressif esptool documentation](https://docs.espressif.com/projects/esptool/en/latest/)
- [Papyrix Reader device support](https://github.com/bigbag/papyrix-reader/blob/main/docs/device-support-matrix.md)

## License

MIT License. See [LICENSE](LICENSE).
