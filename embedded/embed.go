package embedded

import (
	_ "embed"
)

//go:embed bootloader.bin
var bootloader []byte

//go:embed partitions.bin
var partitions []byte

// Bootloader returns the embedded ESP32-C3 bootloader binary.
func Bootloader() []byte {
	return bootloader
}

// Partitions returns the embedded partition table binary.
func Partitions() []byte {
	return partitions
}
