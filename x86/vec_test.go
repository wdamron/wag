package in

import (
	"fmt"
	"github.com/tsavola/wag/buffer"
	"golang.org/x/arch/x86/x86asm"
	"testing"
)

func TestVectorInstructions(t *testing.T) {
	testEncode := func(fn func(*Buf)) x86asm.Inst {
		text := &Buf{Buffer: buffer.NewLimited(nil, 32)}
		fn(text)
		if len(text.Errors) != 0 {
			for err := range text.Errors {
				t.Error(err)
			}
		}
		insn, err := x86asm.Decode(text.Bytes(), 64)
		if err != nil {
			t.Error(err)
		}
		return insn
	}

	checkInst := func(inst x86asm.Inst, op x86asm.Op, args ...string) {
		if inst.Op != op {
			t.Errorf("Found op=%s", inst.Op.String())
		}
		if len(inst.Args) < len(args) {
			t.Errorf("Expected to find %v args, found %v", len(args), len(inst.Args))
		}
		for i, expected := range args {
			found := inst.Args[i]
			if found == nil {
				t.Errorf("Found nil arg at i=%v", i)
			} else if found.String() != expected {
				t.Errorf("Expected to find arg %s at i=%v, found %s", expected, i, found.String())
			}
		}
	}

	for i := 0; i <= 15; i++ {
		for j := 0; j <= 15; j++ {
			xi := fmt.Sprintf("X%d", i)
			xj := fmt.Sprintf("X%d", j)

			// Octet moves:
			checkInst(testEncode(func(text *Buf) { MOVOA.RegReg(text, Reg(i), Reg(j)) }), x86asm.MOVDQA, xi, xj)
			checkInst(testEncode(func(text *Buf) { MOVOU.RegReg(text, Reg(i), Reg(j)) }), x86asm.MOVDQU, xi, xj)
			checkInst(testEncode(func(text *Buf) { MOVOAmr.RegReg(text, Reg(i), Reg(j)) }), x86asm.MOVDQA, xj, xi)
			checkInst(testEncode(func(text *Buf) { MOVOUmr.RegReg(text, Reg(i), Reg(j)) }), x86asm.MOVDQU, xj, xi)

			// Packed shifts with imm8:
			checkInst(testEncode(func(text *Buf) { PSRLi.RegImm8(text, Word, Reg(i), 0x4) }), x86asm.PSRLW, xi, "0x4")
			checkInst(testEncode(func(text *Buf) { PSRLi.RegImm8(text, Long, Reg(i), 0x4) }), x86asm.PSRLD, xi, "0x4")
			checkInst(testEncode(func(text *Buf) { PSRLi.RegImm8(text, Quad, Reg(i), 0x4) }), x86asm.PSRLQ, xi, "0x4")
			checkInst(testEncode(func(text *Buf) { PSRLi.RegImm8(text, Octet, Reg(i), 0x4) }), x86asm.PSRLDQ, xi, "0x4")
			checkInst(testEncode(func(text *Buf) { PSLLi.RegImm8(text, Word, Reg(i), 0x4) }), x86asm.PSLLW, xi, "0x4")
			checkInst(testEncode(func(text *Buf) { PSLLi.RegImm8(text, Long, Reg(i), 0x4) }), x86asm.PSLLD, xi, "0x4")
			checkInst(testEncode(func(text *Buf) { PSLLi.RegImm8(text, Quad, Reg(i), 0x4) }), x86asm.PSLLQ, xi, "0x4")
			checkInst(testEncode(func(text *Buf) { PSLLi.RegImm8(text, Octet, Reg(i), 0x4) }), x86asm.PSLLDQ, xi, "0x4")
			checkInst(testEncode(func(text *Buf) { PSRAi.RegImm8(text, Word, Reg(i), 0x4) }), x86asm.PSRAW, xi, "0x4")
			checkInst(testEncode(func(text *Buf) { PSRAi.RegImm8(text, Long, Reg(i), 0x4) }), x86asm.PSRAD, xi, "0x4")

			// Packed shifts:
			checkInst(testEncode(func(text *Buf) { PSRL.RegReg(text, Word, Reg(i), Reg(j)) }), x86asm.PSRLW, xi, xj)
			checkInst(testEncode(func(text *Buf) { PSRL.RegReg(text, Long, Reg(i), Reg(j)) }), x86asm.PSRLD, xi, xj)
			checkInst(testEncode(func(text *Buf) { PSRL.RegReg(text, Quad, Reg(i), Reg(j)) }), x86asm.PSRLQ, xi, xj)
			checkInst(testEncode(func(text *Buf) { PSLL.RegReg(text, Word, Reg(i), Reg(j)) }), x86asm.PSLLW, xi, xj)
			checkInst(testEncode(func(text *Buf) { PSLL.RegReg(text, Long, Reg(i), Reg(j)) }), x86asm.PSLLD, xi, xj)
			checkInst(testEncode(func(text *Buf) { PSLL.RegReg(text, Quad, Reg(i), Reg(j)) }), x86asm.PSLLQ, xi, xj)
			checkInst(testEncode(func(text *Buf) { PSRA.RegReg(text, Word, Reg(i), Reg(j)) }), x86asm.PSRAW, xi, xj)
			checkInst(testEncode(func(text *Buf) { PSRA.RegReg(text, Long, Reg(i), Reg(j)) }), x86asm.PSRAD, xi, xj)

			// Packed add/subtract/and-not:
			checkInst(testEncode(func(text *Buf) { PADD.RegReg(text, Byte, Reg(i), Reg(j)) }), x86asm.PADDB, xi, xj)
			checkInst(testEncode(func(text *Buf) { PADD.RegReg(text, Word, Reg(i), Reg(j)) }), x86asm.PADDW, xi, xj)
			checkInst(testEncode(func(text *Buf) { PADD.RegReg(text, Long, Reg(i), Reg(j)) }), x86asm.PADDD, xi, xj)
			checkInst(testEncode(func(text *Buf) { PADD.RegReg(text, Quad, Reg(i), Reg(j)) }), x86asm.PADDQ, xi, xj)
			checkInst(testEncode(func(text *Buf) { PSUB.RegReg(text, Byte, Reg(i), Reg(j)) }), x86asm.PSUBB, xi, xj)
			checkInst(testEncode(func(text *Buf) { PSUB.RegReg(text, Word, Reg(i), Reg(j)) }), x86asm.PSUBW, xi, xj)
			checkInst(testEncode(func(text *Buf) { PSUB.RegReg(text, Long, Reg(i), Reg(j)) }), x86asm.PSUBD, xi, xj)
			checkInst(testEncode(func(text *Buf) { PSUB.RegReg(text, Quad, Reg(i), Reg(j)) }), x86asm.PSUBQ, xi, xj)
			checkInst(testEncode(func(text *Buf) { ANDNPSD.RegReg(text, F32, Reg(i), Reg(j)) }), x86asm.ANDNPS, xi, xj)
			checkInst(testEncode(func(text *Buf) { ANDNPSD.RegReg(text, F64, Reg(i), Reg(j)) }), x86asm.ANDNPD, xi, xj)

			// Aligned/unaligned moves:
			checkInst(testEncode(func(text *Buf) { MOVAPSD.RegReg(text, F32, Reg(i), Reg(j)) }), x86asm.MOVAPS, xi, xj)
			checkInst(testEncode(func(text *Buf) { MOVAPSD.RegReg(text, F64, Reg(i), Reg(j)) }), x86asm.MOVAPD, xi, xj)
			checkInst(testEncode(func(text *Buf) { MOVUPSD.RegReg(text, F32, Reg(i), Reg(j)) }), x86asm.MOVUPS, xi, xj)
			checkInst(testEncode(func(text *Buf) { MOVUPSD.RegReg(text, F64, Reg(i), Reg(j)) }), x86asm.MOVUPD, xi, xj)
			checkInst(testEncode(func(text *Buf) { MOVAPSDmr.RegReg(text, F32, Reg(i), Reg(j)) }), x86asm.MOVAPS, xj, xi)
			checkInst(testEncode(func(text *Buf) { MOVAPSDmr.RegReg(text, F64, Reg(i), Reg(j)) }), x86asm.MOVAPD, xj, xi)
			checkInst(testEncode(func(text *Buf) { MOVUPSDmr.RegReg(text, F32, Reg(i), Reg(j)) }), x86asm.MOVUPS, xj, xi)
			checkInst(testEncode(func(text *Buf) { MOVUPSDmr.RegReg(text, F64, Reg(i), Reg(j)) }), x86asm.MOVUPD, xj, xi)

			// Packed signed/unsigned min/max:
			checkInst(testEncode(func(text *Buf) { PMINS.RegReg(text, Byte, Reg(i), Reg(j)) }), x86asm.PMINSB, xi, xj)
			checkInst(testEncode(func(text *Buf) { PMINS.RegReg(text, Word, Reg(i), Reg(j)) }), x86asm.PMINSW, xi, xj)
			checkInst(testEncode(func(text *Buf) { PMINS.RegReg(text, Long, Reg(i), Reg(j)) }), x86asm.PMINSD, xi, xj)
			checkInst(testEncode(func(text *Buf) { PMINU.RegReg(text, Byte, Reg(i), Reg(j)) }), x86asm.PMINUB, xi, xj)
			checkInst(testEncode(func(text *Buf) { PMINU.RegReg(text, Word, Reg(i), Reg(j)) }), x86asm.PMINUW, xi, xj)
			checkInst(testEncode(func(text *Buf) { PMINU.RegReg(text, Long, Reg(i), Reg(j)) }), x86asm.PMINUD, xi, xj)
			checkInst(testEncode(func(text *Buf) { PMAXS.RegReg(text, Byte, Reg(i), Reg(j)) }), x86asm.PMAXSB, xi, xj)
			checkInst(testEncode(func(text *Buf) { PMAXS.RegReg(text, Word, Reg(i), Reg(j)) }), x86asm.PMAXSW, xi, xj)
			checkInst(testEncode(func(text *Buf) { PMAXS.RegReg(text, Long, Reg(i), Reg(j)) }), x86asm.PMAXSD, xi, xj)
			checkInst(testEncode(func(text *Buf) { PMAXU.RegReg(text, Byte, Reg(i), Reg(j)) }), x86asm.PMAXUB, xi, xj)
			checkInst(testEncode(func(text *Buf) { PMAXU.RegReg(text, Word, Reg(i), Reg(j)) }), x86asm.PMAXUW, xi, xj)
			checkInst(testEncode(func(text *Buf) { PMAXU.RegReg(text, Long, Reg(i), Reg(j)) }), x86asm.PMAXUD, xi, xj)

			// Packed blend:
			checkInst(testEncode(func(text *Buf) { PBLENDi.RegRegImm8(text, Word, Reg(i), Reg(j), 0x04) }),
				x86asm.PBLENDW, xi, xj, "0x4")
			checkInst(testEncode(func(text *Buf) { PBLENDi.RegRegImm8(text, Long, Reg(i), Reg(j), 0x04) }),
				x86asm.BLENDPS, xi, xj, "0x4")
			checkInst(testEncode(func(text *Buf) { PBLENDi.RegRegImm8(text, Quad, Reg(i), Reg(j), 0x04) }),
				x86asm.BLENDPD, xi, xj, "0x4")

			// Packed shuffle:
			checkInst(testEncode(func(text *Buf) { PSHUFDi.RegRegImm8(text, Reg(i), Reg(j), 0x04) }),
				x86asm.PSHUFD, xi, xj, "0x4")
			checkInst(testEncode(func(text *Buf) { PSHUFHWi.RegRegImm8(text, Reg(i), Reg(j), 0x04) }),
				x86asm.PSHUFHW, xi, xj, "0x4")
			checkInst(testEncode(func(text *Buf) { PSHUFLWi.RegRegImm8(text, Reg(i), Reg(j), 0x04) }),
				x86asm.PSHUFLW, xi, xj, "0x4")
			checkInst(testEncode(func(text *Buf) { SHUFPDi.RegRegImm8(text, Reg(i), Reg(j), 0x04) }),
				x86asm.SHUFPD, xi, xj, "0x4")
			checkInst(testEncode(func(text *Buf) { SHUFPSi.RegRegImm8(text, Reg(i), Reg(j), 0x04) }),
				x86asm.SHUFPS, xi, xj, "0x4")
		}
	}
}
