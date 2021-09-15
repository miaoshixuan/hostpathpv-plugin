package util

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type CSIIdentifier struct {
	EncodingVersion uint16
	Ephemeral       bool
	VolumeName      string
}

// This maximum comes from the CSI spec on max bytes allowed in the various CSI ID fields.
const maxVolIDLen = 128

const (
	VolIDVersion   uint16 = 1
	knownFieldSize        = 15
)

/*
ComposeCSIID composes a CSI ID from passed in parameters.
version 1 of the encoding scheme is as follows,
	[csi_id_version=1:4byte] + [-:1byte]
	[Ephemeral=1:4bytes] + [-:1byte]
	[length of VolumeName=1:4byte] + [-:1byte]
	[VolumeName:108bytes (MAX)]

	Total of constant field lengths, including '-' field separators would hence be,
	4+1+4+1+4+1 = 15
*/
func (ci CSIIdentifier) ComposeCSIID() (string, error) {
	buf16 := make([]byte, 2)

	if (knownFieldSize + len(ci.VolumeName)) > maxVolIDLen {
		return "", errors.New("CSI ID encoding length overflow")
	}

	binary.BigEndian.PutUint16(buf16, ci.EncodingVersion)
	versionEncodedHex := hex.EncodeToString(buf16)

	binary.BigEndian.PutUint16(buf16, formatBoolToUint16(ci.Ephemeral))
	isEphemeralEncodedHex := hex.EncodeToString(buf16)

	binary.BigEndian.PutUint16(buf16, uint16(len(ci.VolumeName)))
	volumeNameLength := hex.EncodeToString(buf16)

	return strings.Join([]string{versionEncodedHex, isEphemeralEncodedHex, volumeNameLength, ci.VolumeName}, "-"), nil
}

func (ci *CSIIdentifier) DecomposeCSIID(composedCSIID string) (err error) {
	bytesToProcess := uint16(len(composedCSIID))

	// if length is less that expected constant elements, then bail out!
	if bytesToProcess < knownFieldSize {
		return errors.New("failed to decode CSI identifier, string underflow")
	}

	// parse id version
	buf16, err := hex.DecodeString(composedCSIID[0:4])
	if err != nil {
		return err
	}
	ci.EncodingVersion = binary.BigEndian.Uint16(buf16)
	// 4 for version encoding and 1 for '-' separator
	bytesToProcess -= 5

	// parse volume Persistent attr
	buf16, err = hex.DecodeString(composedCSIID[5:9])
	if err != nil {
		return err
	}
	isEphemeral, err := parseBoolFromUint16(binary.BigEndian.Uint16(buf16))
	if err != nil {
		return err
	}
	ci.Ephemeral = isEphemeral
	// 4 for ephemeral encoding and 1 for '-' separator
	bytesToProcess -= 5

	// parse volume name length
	buf16, err = hex.DecodeString(composedCSIID[10:14])
	if err != nil {
		return err
	}
	volumeNameLength := binary.BigEndian.Uint16(buf16)
	// 4 for length encoding and 1 for '-' separator
	bytesToProcess -= 5

	if bytesToProcess != volumeNameLength {
		return errors.New("failed to decode CSI identifier, string size mismatch")
	}
	ci.VolumeName = composedCSIID[15 : 15+volumeNameLength]

	return err
}

func formatBoolToUint16(b bool) uint16 {
	if b {
		return 1
	}
	return 0
}

func parseBoolFromUint16(v uint16) (bool, error) {
	switch v {
	case 0:
		return false, nil
	case 1:
		return true, nil
	}
	return false, fmt.Errorf("parse bool from uint16 %v err", v)
}
