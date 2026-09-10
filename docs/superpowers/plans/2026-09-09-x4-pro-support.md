# X4 Pro Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add safe X4 Pro flashing without removing the existing X3/X4 path, and make the documentation match the shipped commands.

**Architecture:** Keep one Go executable and the existing serial, SLIP, and compressed-transfer code. Identify the ROM before any register or RAM write, check the application target, and select the matching stub and bootloader. Use simple chip-ID switches, not a driver registry or a new transport framework.

**Tech Stack:** Go 1.25.5 as specified by `go.mod`, Cobra, `go.bug.st/serial`, existing Linux raw serial code, Go standard library, embedded firmware assets.

**Spec:** The user requests a recheck and an implementation plan, including documentation. The requirements and evidence below form the design for this plan. Implementation is not authorized by this document alone.

## Global Constraints

- Support the reader's `release_xteink_c3` artifact for X3/X4 and `release_x4pro` artifact for X4 Pro.
- Keep the existing CLI commands `flash` and `info`. Do not add a required device flag.
- Keep flash offsets: bootloader `0x0000`, partitions `0x8000`, OTA data `0xE000`, application `0x10000`.
- Keep the shared 16 MB partition layout. The application slot size is `0x640000` bytes.
- Reject unknown chips, target mismatches, and invalid application images before flash erase or write.
- A firmware target mismatch must also stop watchdog writes and stub upload.
- `--firmware-only` must still identify the chip and check the image. It must skip bootloader, partitions, and OTA-data writes.
- Target unlocked devices. Do not add secure-boot, encrypted-flash, or eFuse-programming support.
- Keep hardware USB Serial/JTAG as the supported native USB path. Do not infer the MCU from USB VID/PID.
- Do not add a Python runtime dependency, an esptool subprocess backend, a manifest requirement, or a network download at flash time.
- Do not claim on-device MD5/readback verification. The current implementation does not perform it.
- Preserve unrelated user changes. Do not commit or publish without a user request.
- Use Go's existing test tooling. No new test framework.

## Recheck Results

### Confirmed target contract

Reader checkout used for the recheck: `/home/work/dev/personal/papyrix-reader`.

| Item | X3/X4 | X4 Pro |
| --- | --- | --- |
| Release environment | `release_xteink_c3` | `release_x4pro` |
| Release application | `papyrix-xteink-c3.bin` | `papyrix-x4pro.bin` |
| MCU/image chip ID | ESP32-C3 / 5 | ESP32-S3 / 9 |
| Flash | 16 MB | 16 MB |
| Reader flash configuration | DIO | QIO, with OPI PSRAM configuration |
| Bootloader offset | `0x0000` | `0x0000` |
| Application offset | `0x10000` | `0x10000` |

Evidence:

- Reader `platformio.ini:22-24,56-60,94-132` defines the layouts and targets.
- Reader `scripts/package_firmware.py:13-30,75-113` copies application-only images. The published `.bin` files are not merged full-flash images.
- Reader `docs/x4pro-specifications.md:39-56` requires unlocked USB and Power held until flashing ends and the application starts.
- Arduino `cores/esp32/HardwareSerial.h:429-436` maps `ARDUINO_USB_MODE=1` to hardware USB Serial/JTAG. The earlier TinyUSB/OTG interpretation was incorrect.
- Both built application images have the ESP-IDF application descriptor magic `0xABCD5432` at offset 32.
- The embedded partition binary and both reader release partition binaries are byte-identical.

Binary inspection on the recheck date:

| Asset | Bytes | Image chip ID | SHA-256 |
| --- | ---: | ---: | --- |
| Reader C3 bootloader | 18672 | 5 | `6e6b0d386095f0783e9f0513ea29ea2ce413790e7f7153a06a2b04e2b5f59fb6` |
| Reader S3 bootloader | 19984 | 9 | `5403ba8cdf81cbb47f2debe13c0f5ff5903540075ccaf3fac65f0ee68213cb7d` |
| Reader C3 application | 4357424 | 5 | `17e9c067d4e113c59af7f3351b03b46ed88b10ff7783f942fb92f6cb28e053c5` |
| Reader S3 application | 4299408 | 9 | `993f94fdb21525339a9eb15e62476b4e3f12bfceef25302e56ae0dee2e1ca2b8` |

These hashes record the inspected builds. They are not constants to impose on future releases.

### Confirmed defects and gaps

1. `ParseSecurityInfo` reads bytes `0:4`, which contain flags. The chip ID is at `12:16` in the 20-byte C3/S3 response.
2. `DecodeResponse` always reads the final two bytes as status. ESP32 ROM replies have a four-byte status trailer; the first two bytes contain status/error and the final two bytes are reserved. Stub replies have a two-byte trailer.
3. `Connect` performs C3 watchdog writes and uploads the C3 stub without identifying the MCU.
4. `--port` bypasses the CLI's existing detection step. Auto-detection also accepts an unknown variant after a chip-query failure.
5. Detection has a separate SYNC/response path and reads the first frame without checking that it belongs to `GET_SECURITY_INFO`.
6. Only the C3 stub and bootloader are embedded. Updating only the firmware file cannot add S3 support.
7. `FlashImageCompressed` accepts an unused `verify` argument. The CLI passes `false`; no verification flag exists.
8. README claims MD5 verification, `--verify=false`, `list`, and `version`. None is available in the current command surface.
9. README's flash sequence omits stub upload and describes SPI commands that the current path does not send.
10. README and CI specify Go 1.22, while `go.mod` requires Go 1.25.5.

A temporary Go program invoked the actual repository functions and produced:

```text
S3 security payload: got chip=0, expected=9, error=<nil>
ROM failure status=[1 5 0 0]: IsSuccess=true, expected=false, error=<nil>
```

The temporary program was removed. `go test ./...` passed before changes. Existing tests therefore do not protect these two wire-format defects.

### Reference version and cautions

Use the reader-local esptool **5.1.2** as the protocol reference:

```text
/home/work/dev/personal/papyrix-reader/.pio/packages/tool-esptoolpy/
```

Do not use the separate `/home/work/.platformio` esptool 4.5.1 as the main reference. The newer S3 implementation handles both RTC WDT and SWD. Its source locations are:

- `esptool/loader.py:541-589,1075-1158`: status and security-info layouts.
- `esptool/targets/esp32s3.py:80-95,330-352`: USB identifiers and watchdog handling.
- `esptool/targets/esp32s3.py:364-388,405-412`: reset details and OTG limits.
- `esptool/reset.py`: hardware USB Serial/JTAG reset sequence.
- `esptool/bin_image.py`: image segments, checksum padding, and appended SHA-256.

The existing C3 stub does not equal either reader-local stub generation or the legacy 4.5.1 asset. Preserve it; do not silently replace it while adding S3.

Implementation recheck: reader-local esptool 5.1.2 uses `STUB_SUBDIRS = ["1"]`. Use its generation-1 S3 stub because this is the reader-proven default. Do not use the generation-2 asset only because it is smaller.

The newer C3 reference also has revision-dependent ROM port-variable addresses. Preserve the existing C3 behavior in this change and test the actual X3/X4 boards. Do not claim support for every C3 silicon revision from the chip-ID check alone.

## Design Decisions

**Recommended:** Add chip selection to the existing native Go path. This preserves the single-binary distribution and shares the existing transfer code.

**Rejected alternatives:** An esptool subprocess adds a runtime and deployment dependency. A firmware-only S3 path still needs an S3 stub and cannot install the correct bootloader. Neither is a smaller complete solution for this tool.

Use `uint32` chip IDs throughout, with named constants for 5 and 9. Use explicit errors for unsupported IDs. Keep `protocol.ChipName` a display helper, not a support check.

Consolidate ROM identification in `internal/flasher`. Detection may import flasher; flasher must not import detection. This removes the duplicate protocol path rather than extending it twice.

The order for flashing must be:

```text
Read and validate application
  -> Find matching device, or use explicit port
  -> Reset, SYNC, and identify ROM on the opened port
  -> Check chip match and security state
  -> Select transport parameters and matching assets
  -> Configure watchdogs and upload matching stub
  -> Write selected regions
  -> Reboot
```

Normal hardware USB Serial/JTAG keeps the existing 6 KB RAM upload and 16 KB compressed-transfer block sizes. An S3 ROM reporting OTG port number 3 must receive a clear unsupported-transport error before writes in this implementation. Do not silently apply Serial/JTAG limits to OTG. OTG needs both 2 KB RAM and stub-flash blocks plus reset handling; it is not required by the inspected reader configuration.

## File Map

| Files | Responsibility |
| --- | --- |
| `internal/protocol/commands.go`, `packet.go` | Chip IDs, status trailers, security-info parsing |
| `internal/protocol/image.go` (new) | Bounded ESP application-image validation |
| `internal/protocol/esp32c3.go` | Shared addresses remain unchanged; do not rename it only for style |
| `internal/flasher/flasher.go` | Shared identification, target gate, watchdog selection, stub state |
| `internal/detect/detect.go` | Port enumeration and matching-chip selection |
| `internal/stub/stub.go`, new S3 JSON and license | Chip-specific RAM executable |
| `embedded/embed.go`, `embedded/bootloader-s3.bin` (new) | Chip-specific bootloaders; one shared partition binary |
| `cmd/papyrix-flasher/main.go` | Image preflight, region selection, help and power instructions |
| `Makefile` | Validated asset refresh from both release builds |
| Existing protocol/stub tests; new image, flasher, and embedded tests | Wire-format, refusal, transition, and asset regressions |
| `README.md`, `CLAUDE.md` | User and maintainer documentation |
| `.github/workflows/build.yml`, `release.yml` | Use the Go version specified by `go.mod`; preserve platform matrix |

## Task 1: Fix ROM Responses and Chip Identification

**Files:** `internal/protocol/packet.go`, `commands.go`, `commands_test.go`, `packet_test.go`; all decoder callers in flasher and detection.

**Interfaces produced:**

```go
const ChipIDESP32S3 = 0x09
const ROMStatusBytes = 4
const StubStatusBytes = 2

type SecurityInfo struct {
    Flags         uint32
    FlashCryptCnt byte
    ChipID        uint32
}

func DecodeResponse(data []byte, statusBytes int) (*Response, error)
func ParseSecurityInfo(data []byte) (*SecurityInfo, error)
```

- [ ] Replace the existing security-info fixtures that encode the chip ID in the flags field. Use a 20-byte payload with nonzero flags that differ from the chip ID. Cover C3, S3, and a truncated payload.
- [ ] Add a regression for a failed ROM reply and retain the successful two-byte stub case. The ROM failure fixture is:

```go
frame := []byte{1, CmdReadReg, 4, 0, 0, 0, 0, 0, 1, 5, 0, 0}
resp, err := DecodeResponse(frame, ROMStatusBytes)
if err != nil { t.Fatal(err) }
if resp.IsSuccess() { t.Fatal("ROM failure accepted as success") }
```

- [ ] Run `go test ./internal/protocol` before the fix and record the behavioral failure. Do not treat a compile error from the new signature as the regression proof.
- [ ] Require `statusBytes` to be 2 or 4. Require the declared body to contain the full trailer. Read status/error from the beginning of that trailer, exclude the entire trailer from `Data`, and preserve length/direction validation.
- [ ] Parse a minimum 20-byte security payload. Read flags at `0:4`, encryption count at byte 4, and chip ID at `12:16`. Permit future trailing fields; do not accept a flags-only or S2-length payload as a chip identity.
- [ ] Rename `ESP32C3Info` to `SecurityInfo` and migrate every caller. No alias. A Go LSP was unavailable during planning; use Go symbol references if available at execution, otherwise use a complete scoped caller search.
- [ ] Use four-byte decoding before the stub greeting and two-byte decoding after it. Update existing decoder tests explicitly; do not guess the trailer length from zero bytes.
- [ ] Run `go test ./internal/protocol ./internal/flasher ./internal/detect`.

**Acceptance:** The recorded S3 payload returns ID 9. The recorded ROM error is not a success. Existing stub responses still decode correctly.

## Task 2: Validate Application Files Before Device Writes

**Files:** Create `internal/protocol/image.go`, `internal/protocol/image_test.go`. Integrate the call in CLI in Task 5.

**Interface produced:**

```go
func ValidateApplication(data []byte) (uint32, error)
```

- [ ] Add a small fixture builder for valid C3/S3 application images. It must build a 24-byte header, a first segment containing `0xABCD5432`, the segment checksum, and an optional appended digest. Do not use installed reader builds as permanent test dependencies.
- [ ] Test valid C3 and S3 images, an unsupported chip, a truncated segment, a bootloader/merged-image input without the application descriptor, an oversized image, a bad checksum, and a bad appended digest. One table and one fixture builder are sufficient.
- [ ] Validate the 24-byte image header, magic `0xE9`, supported chip ID at `12:14`, segment count 1 through 16, and digest flag 0 or 1.
- [ ] Require total file length at most `0x640000`. Check every segment header and length before slicing. Compare lengths against remaining bytes before converting them to `int`.
- [ ] Require the first segment to contain the ESP-IDF application descriptor magic. This deliberately accepts ESP-IDF application images, not arbitrary same-MCU binaries or a full-flash dump. State this limit in README.
- [ ] Walk segment data without copying it. XOR all segment bytes starting from `0xEF`. After the last segment, the checksum byte is at the end of the next 16-byte block:

```go
checksumOffset := ((end + 16) / 16) * 16 - 1
```

- [ ] Check that the checksum byte exists and matches. If header byte 23 is 1, require and verify the following 32-byte SHA-256 over the image through the checksum byte. Permit only trailing `0xFF` padding after the complete image; reject another appended payload.
- [ ] Run `go test ./internal/protocol -run 'TestValidateApplication'`. Also use a temporary program to accept both real reader release applications and reject both reader bootloaders.

**Acceptance:** Invalid files fail without opening a serial port. File validation is described as input validation, not flash readback or authenticity verification.

## Task 3: Add Target Assets and a Safe Refresh Command

**Files:** `internal/stub/stub.go`, `stub_test.go`; create `internal/stub/stub_flasher_32s3.json`, `internal/stub/LICENSE-ESPRESSIF-STUB`; `embedded/embed.go`; create `embedded/bootloader-s3.bin`, `embedded/embed_test.go`; modify `Makefile`.

**Interfaces produced:**

```go
// In package stub:
func Get(chipID uint32) (*Stub, error)

// In package embedded:
func Bootloader(chipID uint32) ([]byte, error)
// Partitions() remains unchanged.
```

- [ ] Keep the existing C3 JSON. Add the S3 generation-1 JSON from reader-local esptool 5.1.2 at `esptool/targets/stub_flasher/1/esp32s3.json`.
- [ ] Record the source, generation, and SHA-256 of the added asset in README. The generation-1 S3 stub includes text and data segments.
- [ ] Select assets with explicit C3/S3 switches and error on any other ID. Keep decoding cached per chip, so `info` scans or repeated calls do not repeatedly allocate decoded segments. Do not share one cache slot between targets.
- [ ] Replace the stub test that requires nonempty data for every stub. Keep tests for target-specific executable address/entry bounds and unknown-ID rejection. Remove the pointer-identity cache test; it does not check device behavior.
- [ ] Copy the S3 release bootloader from `.pio/build/release_x4pro/bootloader.bin`. Keep the C3 filename `embedded/bootloader.bin` and refresh it from `release_xteink_c3`, not `default`.
- [ ] Add embedded-asset tests that decode each selected bootloader header and reject wrong-chip selection. Check the shared partition entries used by the CLI against the fixed addresses and application slot capacity.
- [ ] Change `make update-embedded` to accept `READER_DIR ?= ../papyrix-reader`. Preflight both bootloaders and both partition files before copying any destination. Require nonempty bootloaders with correct image magic/chip IDs and require the two partition binaries to match. Use shell tools already available to the Makefile, such as `test`, `od`, and `cmp`; do not add a Python requirement.
- [ ] Copy only after all preflight checks pass. A missing release environment or partition mismatch must exit nonzero without changing checked-in assets. Add `update-embedded` to `.PHONY`.
- [ ] Test `make update-embedded READER_DIR=/home/work/dev/personal/papyrix-reader`, plus missing-environment and differing-partition cases in temporary directories. Restore temporary test inputs; never alter the reader builds.
- [ ] Run `go test ./internal/stub ./embedded`.

**Acceptance:** A normal build needs no reader checkout and no download. Refreshing from a reader checkout cannot silently select a dev bootloader or mismatched partition tables.

## Task 4: Share Identification and Add the Pre-write Target Gate

**Files:** `internal/flasher/flasher.go`, `internal/detect/detect.go`; create `internal/flasher/flasher_test.go` for buffered-response and rejection regressions.

**Interfaces produced:**

```go
func (f *Flasher) Identify() (*protocol.SecurityInfo, error)
func (f *Flasher) Connect(expectedChipID uint32) error
func DetectDevice(baudRate int, expectedChipID uint32) (*Result, error)
// DetectOnPort and ListDevices keep their current signatures.
```

- [ ] Extract reset, buffer/state clearing, SYNC, and `GET_SECURITY_INFO` into `Identify`. It must not disable watchdogs, upload a stub, erase flash, or reboot into an application.
- [ ] Route `detect.tryPort` through `flasher.New(port).Identify()`. Delete detection's duplicate SYNC and chip-query functions. Remove the successful unknown-variant fallback.
- [ ] Require matching response opcodes in the shared receive path. Skip delayed SYNC replies under the original deadline, rather than accepting them as security information or register results. Preserve pending `OHAI` handling.
- [ ] Add tests using the existing in-memory receive buffer for a stale SYNC followed by security info, a failed ROM reply, and the transition to two-byte stub replies. Assert interpreted results, not private field copies.
- [ ] Have `Connect(expectedChipID)` call `Identify` on the currently opened port. Reject an unsupported chip or mismatch before any `WRITE_REG`, `MEM_BEGIN`, or flash command.
- [ ] Reject flashing when secure boot or secure download is enabled (`Flags & 0x5 != 0`), or when the set-bit count of `FlashCryptCnt` is odd. Use `math/bits.OnesCount8`. `info` may report the identity without attempting to change the device.
- [ ] Make auto-detection continue past a supported but nonmatching MCU. If no matching device is found, return an error that includes the expected chip. Keep the explicit-port mismatch error distinct from a connection failure.
- [ ] Select watchdog addresses by the identified MCU. Share the write sequence and keys, not the chip-dependent addresses:

| Register/value | C3 current path | S3 |
| --- | --- | --- |
| ROM port variable | `0x3FCDF07C` | `0x3FCEF14C` |
| USB Serial/JTAG port number | 3 | 4 |
| RTC WDT configuration | `0x60008090` | `0x60008098` |
| RTC WDT write protection | `0x600080A8` | `0x600080B0` |
| SWD configuration | `0x600080AC` | `0x600080B4` |
| SWD write protection | `0x600080B0` | `0x600080B8` |

- [ ] For S3, fail if reading the active port fails. Reject OTG port number 3 before writes. For Serial/JTAG port 4, disable RTC WDT and enable SWD auto-feed using the S3 registers. Preserve the existing C3 behavior and do not probe both C3 ROM addresses by guessing.
- [ ] Select `stub.Get(info.ChipID)` only after all checks. Preserve optional data-segment handling. Set stub-running state only after a valid `OHAI` greeting.
- [ ] Migrate the sole CLI `Connect` call and all test callers. Do not leave an unvalidated compatibility overload.
- [ ] Run `go test ./internal/flasher ./internal/detect ./internal/protocol ./internal/stub`.

**Acceptance:** Both explicit-port and automatic flashing use the same target gate. No unknown or mismatched MCU reaches chip-specific writes. `info` performs identification without loading a stub.

## Task 5: Integrate the CLI and Prove the Real Flow

**Files:** `cmd/papyrix-flasher/main.go`, `internal/flasher/flasher.go`. Serial files change only if hardware evidence identifies a reset defect.

- [ ] Call `protocol.ValidateApplication` immediately after reading the file. Use its returned chip ID for auto-detection and `Connect`.
- [ ] Print the X4 Pro power instruction before auto-detection or reset when the input image targets S3. The instruction is too late if printed after the chip has already lost its power latch. Print an equivalent generic power note before `info` starts probing.
- [ ] After `Connect` succeeds, select the matching embedded bootloader and construct regions. Full flash writes bootloader, shared partitions, erased OTA data, and application. Firmware-only writes just the application.
- [ ] Remove the unused `verify` parameter from `FlashImageCompressed` and its sole production caller. Do not add MD5 support just to preserve stale documentation.
- [ ] Update root and flag help for X3/X4 and X4 Pro. State that firmware-only also leaves OTA state unchanged.
- [ ] Keep the current serial reset sequence for the initial hardware comparison. Do not make an OTG-specific change based on the earlier incorrect USB assumption. Compare failed reset traces with esptool 5.1.2 `USBJTAGSerialReset` before changing either `serial.go` or `serial_linux.go`; update both platform paths if a shared sequence change is required.
- [ ] Build and inspect the actual CLI:

```sh
make build
bin/papyrix-flasher --help
bin/papyrix-flasher flash --help
bin/papyrix-flasher info --help
```

- [ ] Use temporary malformed inputs and wrong-target application fixtures to prove preflight/refusal. Keep a focused regression for rejection before write commands; do not retain a general mock transport framework.
- [ ] On an explicitly selected X4 Pro, with Power held, exercise `info`, full flash, firmware-only, reboot, and application startup. Repeat identification and flashing after the application has booted. Record the active ROM port and detected chip.
- [ ] On an X3 or X4, exercise the same C3 paths. Include both explicit port and auto-detection across the hardware checks. With both devices connected, prove that auto-detection chooses the image's MCU.
- [ ] Try a C3 image against an explicit S3 port and an S3 image against an explicit C3 port. Observe rejection before register/RAM/flash writes. Confirm the existing application remains intact after a manual restart.
- [ ] Verify written regions independently with esptool readback or `verify-flash`, not this flasher's completion message. For firmware-only, compare bootloader, partition, and OTA-data contents before and after.
- [ ] Record host OS, device, reader build, command, and result. Cross-compilation is not serial-driver or hardware verification. Do not claim Windows/macOS hardware validation from a Linux run.

**Hardware gate:** No device was flashed during planning. If suitable hardware is unavailable during implementation, complete software work and report exactly which scenarios remain unverified. Do not mark end-to-end X4 Pro support verified until the full flash and application startup checks pass.

## Task 6: Update User, Maintainer, and Release Documentation

**Files:** `README.md`, `CLAUDE.md`, `.github/workflows/build.yml`, `.github/workflows/release.yml`, and the stub license file from Task 3.

Do this after the smoke checks so that documentation records actual behavior. No separate documentation site is needed.

- [ ] Update README introduction, supported-device table, and feature list. Map X3/X4 to C3 and `papyrix-xteink-c3.bin`; map X4 Pro to S3 and `papyrix-x4pro.bin`.
- [ ] Replace the claim that any C3 firmware works with the exact application-format, chip, and partition-layout requirements. Explain that chip identity is not proof of board, panel, PSRAM, or firmware compatibility.
- [ ] Include these commands with the real artifact names:

```sh
papyrix-flasher flash papyrix-xteink-c3.bin
papyrix-flasher flash -p /dev/ttyACM0 papyrix-x4pro.bin
papyrix-flasher flash --firmware-only -p /dev/ttyACM0 papyrix-x4pro.bin
papyrix-flasher info -p /dev/ttyACM0
```

- [ ] Add an X4 Pro installation section: unlocked USB, data-capable cable, close the serial monitor, hold Power before detection/reset, release it only after the application starts. Do not invent a manual button sequence not established by the reader documentation or hardware check.
- [ ] Explain full-flash versus firmware-only behavior, including OTA-data reset only in full-flash mode. State that full flash replaces the partition table and bootloader; users with a custom layout must not assume compatibility.
- [ ] Remove `--verify=false`, `list`, and `version` examples. Replace port-list troubleshooting with supported `info` usage and OS port inspection. Remove MD5/default-verification claims. Distinguish file checksum/digest checks from device readback.
- [ ] Correct the layout table, including `app0` capacity `0x640000` and the coredump region at `0xFF0000`. Remove the obsolete fixed 12 KB bootloader size.
- [ ] Replace the flash-sequence section with identification, safety gates, chip-specific watchdog/stub selection, compressed writes, and reboot. Describe native USB Serial/JTAG rather than a universal transistor/GPIO0 circuit. Record the actual RAM/stub block sizes and unsupported OTG limit.
- [ ] Update the source tree to show the stub package, image validation, and S3 bootloader. Document the S3 stub source, version, generation, and hash.
- [ ] Document reproducible maintenance commands:

```sh
# In papyrix-reader:
make release
# In papyrix-flasher:
make update-embedded
make test
make build-all
```

- [ ] Update the Go badge and build prerequisite to match `go.mod`. In both GitHub workflows, replace hardcoded Go 1.22 setup with `go-version-file: go.mod`. Keep the existing OS/architecture matrix.
- [ ] Keep a source notice next to the S3 stub. Do not add release-archive licensing machinery for this implementation.
- [ ] Update `CLAUDE.md` architecture, target constants, asset-refresh instructions, compressed stub flow, and verification limits. Remove its obsolete C3-only and MD5 statements.
- [ ] Use the existing generated GitHub release notes. Provide a release-note entry for X4 Pro support, image mismatch refusal, protocol error fixes, required Power hold, and hardware verification status. Do not create a new changelog system solely for this change.
- [ ] Check every documented command and flag against actual `--help` output. Do not mark an unexecuted hardware example as tested.

**Acceptance:** README and maintainer instructions agree with the binary. Users can select the correct artifact, understand which regions change, update embedded assets, and see the verification limits.

## Final Verification and Handoff

- [ ] Run formatting and project-wide validation once after integration:

```sh
make fmt
go test ./...
go vet ./...
make build-all
```

- [ ] Check release archives contain the executable and required S3 stub license notice. Confirm the normal build still has no dependency on a sibling checkout.
- [ ] Re-run the two recorded protocol reproductions through the regression tests. Require chip ID 9 and rejection of the ROM failure.
- [ ] Re-run valid reader image input checks and invalid/mismatched input checks after final integration.
- [ ] Confirm every changed exported signature has no obsolete caller or compatibility alias.
- [ ] Complete the hardware record from Task 5 or list the unverified scenarios explicitly. Input tests and compilation cannot substitute for these checks.
- [ ] After a successful smoke test, remove temporary probes and refresh-test directories. Keep only the regression tests that protect observable failures.
- [ ] Review docs, asset provenance, license packaging, and release-note content together with the actual CLI behavior.

## Hardware Verification Record

Verified on 2026-09-10 with an Xteink X4 Pro connected through hardware USB Serial/JTAG at `/dev/ttyACM0`.

- Firmware: Papyrix Reader v1.29.0 `papyrix-x4pro.bin`
- SHA-256: `993f94fdb21525339a9eb15e62476b4e3f12bfceef25302e56ae0dee2e1ca2b8`
- `info` result: ESP32-S3, chip ID `0x09`
- Full flash result: bootloader, partition table, OTA state, and application commands completed
- Reboot result: the application screen appeared while Power remained held

This run did not perform independent flash readback. It did not exercise firmware-only mode or C3 hardware.

## Plan Coverage Review

- Target detection, response trailers, stale frames, and explicit-port safety: Tasks 1 and 4.
- Firmware format, size, target, checksum, and digest checks: Tasks 2 and 5.
- Chip-specific executable assets, watchdogs, shared partitions, and refresh safety: Tasks 3 and 4.
- Existing C3 behavior, normal S3 USB path, full/firmware-only modes, and reboot: Task 5.
- User instructions, maintainer context, misleading commands, toolchain version, license distribution, and release notes: Task 6.
- Software proof, actual device proof, and truthful verification claims: Task 5 and final verification.

This plan is ready for user review. No production implementation changes are included in the planning deliverable.
