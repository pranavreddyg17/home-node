package backup

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestCleanExt4HeaderRefusesRecoveryAndErrors(t *testing.T) {
	valid := make([]byte, 2048)
	binary.LittleEndian.PutUint16(valid[1024+0x38:], 0xef53)
	binary.LittleEndian.PutUint16(valid[1024+0x3a:], 1)
	for _, scenario := range []string{"clean", "dirty", "errors", "orphan-recovery", "journal-recovery", "wrong-magic", "truncated"} {
		t.Run(scenario, func(t *testing.T) {
			data := bytes.Clone(valid)
			switch scenario {
			case "dirty":
				binary.LittleEndian.PutUint16(data[1024+0x3a:], 0)
			case "errors":
				binary.LittleEndian.PutUint16(data[1024+0x3a:], 3)
			case "orphan-recovery":
				binary.LittleEndian.PutUint16(data[1024+0x3a:], 5)
			case "journal-recovery":
				binary.LittleEndian.PutUint32(data[1024+0x60:], 4)
			case "wrong-magic":
				data[1024+0x38] = 0
			case "truncated":
				data = data[:2047]
			}
			err := requireCleanExt4Header(bytes.NewReader(data))
			if (err == nil) != (scenario == "clean") {
				t.Fatal("unexpected header admission", err)
			}
		})
	}
}
