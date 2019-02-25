// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package in

import (
	"encoding/binary"
	"github.com/pkg/errors"
	"math/bits"
)

var nops = [4][4]byte{
	1: {0x90},
	2: {0x66, 0x90},
	3: {0x0f, 0x1f, 0x00},
}

func typeScalarPrefix(t Type) byte { return byte(t)>>2 | 0xf2 } // 0xf3 or 0xf2
func typeRMISizeCode(t Type) byte  { return byte(t)>>3 | 0x0a } // 0x0a or 0x0b

func addrDisp(currentAddr, insnSize, targetAddr int32) int32 {
	if targetAddr != 0 {
		siteAddr := currentAddr + insnSize
		return targetAddr - siteAddr
	} else {
		return -insnSize // infinite loop as placeholder
	}
}

type output struct {
	buf    [16]byte
	offset uint8
}

func (o *output) len() int           { return int(o.offset) }
func (o *output) copy(target []byte) { copy(target, o.buf[:o.offset]) }
func (o *output) debugPrint()        { debugPrintInsn(o.buf[:o.offset]) }

func (o *output) byte(b byte) {
	o.buf[o.offset] = b
	o.offset++
}

func (o *output) byteIf(b byte, condition bool) {
	o.buf[o.offset] = b
	o.offset += bit(condition)
}

// word appends the two bytes of a big-endian word.
func (o *output) word(w uint16) {
	binary.BigEndian.PutUint16(o.buf[o.offset:], w)
	o.offset += 2
}

func (o *output) rex(wrxb rexWRXB) {
	o.buf[o.offset] = Rex | byte(wrxb)
	o.offset++
}

func (o *output) rexIf(wrxb rexWRXB) {
	o.buf[o.offset] = Rex | byte(wrxb)
	o.offset += bit(wrxb != 0)
}

func (o *output) mod(mod Mod, ro ModRO, rm ModRM) {
	o.buf[o.offset] = byte(mod) | byte(ro) | byte(rm)
	o.offset++
}

func (o *output) sib(s Scale, i Index, b Base) {
	o.buf[o.offset] = byte(s) | byte(i) | byte(b)
	o.offset++
}

func (o *output) int8(val int8) {
	o.buf[o.offset] = uint8(val)
	o.offset++
}

func (o *output) int16(val int16) {
	binary.LittleEndian.PutUint16(o.buf[o.offset:], uint16(val))
	o.offset += 2
}

func (o *output) int32(val int32) {
	binary.LittleEndian.PutUint32(o.buf[o.offset:], uint32(val))
	o.offset += 4
}

func (o *output) int64(val int64) {
	binary.LittleEndian.PutUint64(o.buf[o.offset:], uint64(val))
	o.offset += 8
}

func (o *output) int(val int32, size uint8) {
	// Little-endian byte order works for any size
	binary.LittleEndian.PutUint32(o.buf[o.offset:], uint32(val))
	o.offset += size
}

// NP

type NP byte

func (op NP) Type(text *Buf, t Type) {
	var o output
	o.rexIf(typeRexW(t))
	o.byte(byte(op))
	o.copy(text.Extend(o.len()))
}

func (op NP) Simple(text *Buf) {
	text.PutByte(byte(op))
}

// NP with fixed 0xf3 prefix

type NPprefix byte

func (op NPprefix) Simple(text *Buf) {
	var o output
	o.byte(0xf3)
	o.byte(byte(op))
	o.copy(text.Extend(o.len()))
}

// O

type O byte

func (op O) Reg(text *Buf, r Reg) { text.PutByte(byte(op) + byte(r)) }

// M

type M uint16 // opcode byte and ModRO byte

func (op M) Reg(text *Buf, t Type, r Reg) {
	var o output
	o.rexIf(typeRexW(t) | regRexB(r))
	o.byte(byte(op >> 8))
	o.mod(ModReg, ModRO(op), regRM(r))
	o.copy(text.Extend(o.len()))
}

// M instructions which require rex byte with register operand

type Mex2 uint16 // two opcode bytes

func (op Mex2) OneSizeReg(text *Buf, r Reg) {
	var o output
	o.rex(regRexB(r))
	o.word(uint16(op))
	o.mod(ModReg, 0, regRM(r))
	o.copy(text.Extend(o.len()))
}

// RM (MR)

type RM byte    // opcode byte
type RM2 uint16 // two opcode bytes

func (op RM) RegReg(text *Buf, t Type, r, r2 Reg) {
	var o output
	o.rexIf(typeRexW(t) | regRexR(r) | regRexB(r2))
	o.byte(byte(op))
	o.mod(ModReg, regRO(r), regRM(r2))
	o.copy(text.Extend(o.len()))
}

func (op RM2) RegReg(text *Buf, t Type, r, r2 Reg) {
	var o output
	o.rexIf(typeRexW(t) | regRexR(r) | regRexB(r2))
	o.word(uint16(op))
	o.mod(ModReg, regRO(r), regRM(r2))
	o.copy(text.Extend(o.len()))
}

func (op RM) RegMemDisp(text *Buf, t Type, r, base Reg, disp int32) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.rexIf(typeRexW(t) | regRexR(r) | regRexB(base))
	o.byte(byte(op))
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.copy(text.Extend(o.len()))
}

func (op RM2) RegMemDisp(text *Buf, t Type, r, base Reg, disp int32) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.rexIf(typeRexW(t) | regRexR(r) | regRexB(base))
	o.word(uint16(op))
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.copy(text.Extend(o.len()))
}

func (op RM) RegMemIndexDisp(text *Buf, t Type, r, base Reg, index Reg, s Scale, disp int32) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.rexIf(typeRexW(t) | regRexR(r) | regRexX(index) | regRexB(base))
	o.byte(byte(op))
	o.mod(mod, regRO(r), ModRMSIB)
	o.sib(s, regIndex(index), regBase(base))
	o.int(disp, dispSize)
	o.copy(text.Extend(o.len()))
}

// RM (MR) with prefix and two opcode bytes (first byte hardcoded)

type RMprefix uint16    // fixed-length prefix and second opcode byte
type RMprefixnt uint16  // fixed-length prefix and second opcode byte; single data-size (no type)
type RMscalar byte      // second opcode byte; type-dependent fixed-length prefix
type RMpacked byte      // second opcode byte; type-dependent variable-length prefix
type RMpackedsz uint32  // op-code set for B/W/L/Q elements; 0x66 prefix without type-dependent REX.W
type RMIpackedsz string // op-code set for B/W/L/Q/DQ elements with RO; 0x66 prefix without type-dependent REX.W; imm8
type Pminmax string     // placeholder for PMIN/PMAX instructions
type PBlendi uint32     // placeholder for BLEND instructions with imm8
type PShufi string      // placeholder for SHUF instructions with imm8

func (op RMprefix) RegReg(text *Buf, t Type, r, r2 Reg) {
	var o output
	o.byte(byte(op >> 8))
	o.rexIf(typeRexW(t) | regRexR(r) | regRexB(r2))
	o.byte(0x0f)
	o.byte(byte(op))
	o.mod(ModReg, regRO(r), regRM(r2))
	o.copy(text.Extend(o.len()))
}

func (op RMprefixnt) RegReg(text *Buf, r, r2 Reg) {
	var o output
	o.byte(byte(op >> 8))
	o.rexIf(regRexR(r) | regRexB(r2))
	o.byte(0x0f)
	o.byte(byte(op))
	o.mod(ModReg, regRO(r), regRM(r2))
	o.copy(text.Extend(o.len()))
}

func (op RMscalar) RegReg(text *Buf, t Type, r, r2 Reg) {
	var o output
	o.byte(typeScalarPrefix(t))
	o.rexIf(regRexR(r) | regRexB(r2))
	o.byte(0x0f)
	o.byte(byte(op))
	o.mod(ModReg, regRO(r), regRM(r2))
	o.copy(text.Extend(o.len()))
}

func (op RMpacked) RegReg(text *Buf, t Type, r, r2 Reg) {
	var o output
	o.byteIf(0x66, t&8 == 8)
	o.rexIf(regRexR(r) | regRexB(r2))
	o.byte(0x0f)
	o.byte(byte(op))
	o.mod(ModReg, regRO(r), regRM(r2))
	o.copy(text.Extend(o.len()))
}

func (op RMpackedsz) opByte(sz Size) (b byte, ok bool) {
	b = byte(uint64(op) >> (8 * uint64(bits.TrailingZeros8(uint8(sz)))))
	ok = b != 0 && sz <= Quad
	return
}

func (op RMpackedsz) RegReg(text *Buf, sz Size, r, r2 Reg) {
	var o output
	bop, ok := op.opByte(sz)
	if !ok {
		text.Err(errors.Errorf("missing encoding for RMpackedsz op=%x size=%v addr=%v", op, sz, text.Addr))
		return
	}
	o.byte(0x66)
	o.rexIf(regRexR(r) | regRexB(r2))
	o.byte(0x0f)
	o.byte(bop)
	o.mod(ModReg, regRO(r), regRM(r2))
	o.copy(text.Extend(o.len()))
}

func (op Pminmax) opWord(sz Size) (w uint16, ok bool) {
	offset := bits.TrailingZeros8(uint8(sz))
	if offset >= len(op)/2 {
		return
	}
	w = uint16(op[offset*2]) | (uint16(op[offset*2+1])<<8)
	ok = w != 0 && sz <= Long
	return
}

func (op Pminmax) RegReg(text *Buf, sz Size, r, r2 Reg) {
	var o output
	w, ok := op.opWord(sz)
	if !ok {
		text.Err(errors.Errorf("missing encoding for Pminmax op=%x size=%v addr=%v", op, sz, text.Addr))
		return
	}
	o.byte(0x66)
	o.rexIf(regRexR(r) | regRexB(r2))
	o.byte(0x0f)
	o.byte(byte(w))
	w >>= 8
	o.byteIf(byte(w), byte(w) != 0)
	o.mod(ModReg, regRO(r), regRM(r2))
	o.copy(text.Extend(o.len()))
}

func (op RMIpackedsz) opRoBytes(sz Size) (b, ro byte, ok bool) {
	offset := bits.TrailingZeros8(uint8(sz)) & 0xf
	b = op[offset]
	ro = op[offset+5]<<opcodeBase
	ok = b != 0 && sz <= Octet
	return
}

func (op RMIpackedsz) RegImm8(text *Buf, sz Size, r Reg, val int8) {
	var o output
	b, ro, ok := op.opRoBytes(sz)
	if !ok {
		text.Err(errors.Errorf("missing encoding for RMIpackedsz op=%x size=%v addr=%v", op, sz, text.Addr))
		return
	}
	o.byte(0x66)
	o.rexIf(regRexB(r))
	o.byte(0x0f)
	o.byte(b)
	o.mod(ModReg, ModRO(ro), regRM(r))
	o.int8(val)
	o.copy(text.Extend(o.len()))
}

func (op PBlendi) opByte(sz Size) (b byte, ok bool) {
	b = byte(uint32(op) >> (8 * uint64(bits.TrailingZeros8(uint8(sz)))))
	ok = b != 0 && sz >= Word && sz <= Quad
	return
}

func (op PBlendi) RegRegImm8(text *Buf, sz Size, r, r2 Reg, val int8) {
	var o output
	b, ok := op.opByte(sz)
	if !ok {
		text.Err(errors.Errorf("missing encoding for PBlendi op=%x size=%v addr=%v", op, sz, text.Addr))
		return
	}
	o.byte(0x66)
	o.rexIf(regRexR(r) | regRexB(r2))
	o.byte(0x0f)
	o.byte(0x3a)
	o.byte(b)
	o.mod(ModReg, regRO(r), regRM(r2))
	o.int8(val)
	o.copy(text.Extend(o.len()))
}

func (op PShufi) RegRegImm8(text *Buf, r, r2 Reg, val int8) {
	var o output
	o.byteIf(op[0], op[0] != 0x0f)
	o.rexIf(regRexR(r) | regRexB(r2))
	o.byteIf(0x0f, op[0] == 0x0f)
	for i := 1; i < len(op); i++ {
		o.byte(op[i])
	}
	o.mod(ModReg, regRO(r), regRM(r2))
	o.int8(val)
	o.copy(text.Extend(o.len()))
}

func (op RMscalar) TypeRegReg(text *Buf, floatType, intType Type, r, r2 Reg) {
	var o output
	o.byte(typeScalarPrefix(floatType))
	o.rexIf(typeRexW(intType) | regRexR(r) | regRexB(r2))
	o.byte(0x0f)
	o.byte(byte(op))
	o.mod(ModReg, regRO(r), regRM(r2))
	o.copy(text.Extend(o.len()))
}

func (op RMprefix) RegMemDisp(text *Buf, t Type, r, base Reg, disp int32) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.byte(byte(op >> 8))
	o.rexIf(typeRexW(t) | regRexR(r) | regRexB(base))
	o.byte(0x0f)
	o.byte(byte(op))
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.copy(text.Extend(o.len()))
}

func (op RMprefixnt) RegMemDisp(text *Buf, r, base Reg, disp int32) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.byte(byte(op >> 8))
	o.rexIf(regRexR(r) | regRexB(base))
	o.byte(0x0f)
	o.byte(byte(op))
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.copy(text.Extend(o.len()))
}

func (op RMscalar) RegMemDisp(text *Buf, t Type, r, base Reg, disp int32) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.byte(typeScalarPrefix(t))
	o.rexIf(regRexR(r) | regRexB(base))
	o.byte(0x0f)
	o.byte(byte(op))
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.copy(text.Extend(o.len()))
}

func (op RMpacked) RegMemDisp(text *Buf, t Type, r, base Reg, disp int32) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.byteIf(0x66, t&8 == 8)
	o.rexIf(regRexR(r) | regRexB(base))
	o.byte(0x0f)
	o.byte(byte(op))
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.copy(text.Extend(o.len()))
}

func (op RMpackedsz) RegMemDisp(text *Buf, sz Size, r, base Reg, disp int32) {
	var mod, dispSize = dispModSize(disp)
	var o output
	bop, ok := op.opByte(sz)
	if !ok {
		text.Err(errors.Errorf("missing encoding for RMpackedsz op=%x size=%v addr=%v", op, sz, text.Addr))
		return
	}
	o.byte(0x66)
	o.rexIf(regRexR(r) | regRexB(base))
	o.byte(0x0f)
	o.byte(bop)
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.copy(text.Extend(o.len()))
}

func (op PBlendi) RegMemDispImm8(text *Buf, sz Size, r, base Reg, disp int32, val int8) {
	var mod, dispSize = dispModSize(disp)
	var o output
	b, ok := op.opByte(sz)
	if !ok {
		text.Err(errors.Errorf("missing encoding for PBlendi op=%x size=%v addr=%v", op, sz, text.Addr))
		return
	}
	o.byte(0x66)
	o.rexIf(regRexR(r) | regRexB(base))
	o.byte(0x0f)
	o.byte(0x3a)
	o.byte(b)
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.int8(val)
	o.copy(text.Extend(o.len()))
}

func (op PShufi) RegMemDispImm8(text *Buf, r, base Reg, disp int32, val int8) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.byteIf(op[0], op[0] != 0x0f)
	o.rexIf(regRexR(r) | regRexB(base))
	o.byteIf(op[0], op[0] == 0x0f)
	for i := 1; i < len(op); i++ {
		o.byte(op[i])
	}
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.int8(val)
	o.copy(text.Extend(o.len()))
}

func (op Pminmax) RegMemDisp(text *Buf, sz Size, r, base Reg, disp int32) {
	var mod, dispSize = dispModSize(disp)
	var o output
	w, ok := op.opWord(sz)
	if !ok {
		text.Err(errors.Errorf("missing encoding for Pminmax op=%x size=%v addr=%v", op, sz, text.Addr))
		return
	}
	o.byte(0x66)
	o.rexIf(regRexR(r) | regRexB(base))
	o.byte(0x0f)
	o.byte(byte(w))
	w >>= 8
	o.byteIf(byte(w), w != 0)
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.copy(text.Extend(o.len()))
}

// RM instructions with 8-bit operand size

type RMdata8 byte // opcode byte

func (op RMdata8) RegMemDisp(text *Buf, _ Type, r, base Reg, disp int32) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.rex(regRexR(r) | regRexB(base))
	o.byte(byte(op))
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.copy(text.Extend(o.len()))
}

// RM instructions with 16-bit operand size

type RMdata16 byte // opcode byte

func (op RMdata16) RegMemDisp(text *Buf, _ Type, r, base Reg, disp int32) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.byte(0x66)
	o.rexIf(regRexR(r) | regRexB(base))
	o.byte(byte(op))
	o.mod(mod, regRO(r), regRM(base))
	o.int(disp, dispSize)
	o.copy(text.Extend(o.len()))
}

// I

type Ipush byte // opcode of instruction variant with 8-bit immediate

func (op Ipush) Imm(text *Buf, val int32) {
	var valSize = immSize(val)
	var o output
	o.byte(byte(op) &^ (valSize >> 1)) // 0x6a => 0x68 if 32-bit
	o.int(val, valSize)
	o.copy(text.Extend(o.len()))
}

// OI

type OI byte

func (op OI) RegImm64(text *Buf, r Reg, val int64) {
	var o output
	o.rex(RexW | regRexB(r))
	o.byte(byte(op) + byte(r)&7)
	o.int64(val)
	o.copy(text.Extend(o.len()))
}

// MI instructions with varying operand and immediate sizes

type MI uint32 // opcode bytes for 32-bit value and 8-bit value; and common ModRO byte

func (ops MI) RegImm(text *Buf, t Type, r Reg, val int32) {
	var op, valSize = immOpcodeSize(uint16(ops>>8), val)
	var o output
	o.rexIf(typeRexW(t) | regRexB(r))
	o.byte(op)
	o.mod(ModReg, ModRO(ops), regRM(r))
	o.int(val, valSize)
	o.copy(text.Extend(o.len()))
}

func (op MI) RegImm8(text *Buf, t Type, r Reg, val int8) {
	var o output
	o.rexIf(typeRexW(t) | regRexB(r))
	o.byte(byte(op >> 8))
	o.mod(ModReg, ModRO(op), regRM(r))
	o.int8(val)
	o.copy(text.Extend(o.len()))
}

func (op MI) RegImm32(text *Buf, t Type, r Reg, val int32) {
	var o output
	o.rexIf(typeRexW(t) | regRexB(r))
	o.byte(byte(op >> 16))
	o.mod(ModReg, ModRO(op), regRM(r))
	o.int32(val)
	o.copy(text.Extend(o.len()))
}

// MI instructions with 8-bit operand size implementing generic interface

type MI8 uint16 // opcode byte and ModRO byte

func (op MI8) OneSizeRegImm(text *Buf, r Reg, val8 int64) {
	var o output
	o.rex(regRexB(r))
	o.byte(byte(op >> 8))
	o.mod(ModReg, ModRO(op), regRM(r))
	o.int8(int8(val8))
	o.copy(text.Extend(o.len()))
}

// MemDispImm ignores the type argument.
func (op MI8) MemDispImm(text *Buf, _ Type, base Reg, disp int32, val8 int64) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.rexIf(regRexB(base))
	o.byte(byte(op >> 8))
	o.mod(mod, ModRO(op), regRM(base))
	o.int(disp, dispSize)
	o.int8(int8(val8))
	o.copy(text.Extend(o.len()))
}

// MI instructions with 16-bit operand size implementing generic interface

type MI16 uint16 // opcode byte and ModRO byte

// MemDispImm ignores the type argument.
func (op MI16) MemDispImm(text *Buf, _ Type, base Reg, disp int32, val16 int64) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.byte(0x66)
	o.rexIf(regRexB(base))
	o.byte(byte(op >> 8))
	o.mod(mod, ModRO(op), regRM(base))
	o.int(disp, dispSize)
	o.int16(int16(val16))
	o.copy(text.Extend(o.len()))
}

// MI instructions with 32-bit immediate implementing generic interface

type MI32 uint16 // opcode byte and ModRO byte

func (op MI32) MemDispImm(text *Buf, t Type, base Reg, disp int32, val32 int64) {
	var mod, dispSize = dispModSize(disp)
	var o output
	o.rexIf(typeRexW(t) | regRexB(base))
	o.byte(byte(op >> 8))
	o.mod(mod, ModRO(op), regRM(base))
	o.int(disp, dispSize)
	o.int32(int32(val32))
	o.copy(text.Extend(o.len()))
}

// RMI

type RMI byte // opcode of 8-bit variant, transformed to 32-bit variant automatically

func (op RMI) RegRegImm(text *Buf, t Type, r, r2 Reg, val int32) {
	var valSize = immSize(val)
	var o output
	o.rexIf(typeRexW(t) | regRexR(r) | regRexB(r2))
	o.byte(byte(op) &^ (valSize >> 1)) // 0x6b => 0x69 if 32-bit
	o.mod(ModReg, regRO(r), regRM(r2))
	o.int(val, valSize)
	o.copy(text.Extend(o.len()))
}

// RMI with prefix, two opcode bytes (first byte hardcoded) and size code

type RMIscalar byte // opcode of 8-bit variant, transformed to 32-bit variant automatically

func (op RMIscalar) RegRegImm8(text *Buf, t Type, r, r2 Reg, val int8) {
	var o output
	o.byte(0x66)
	o.byte(0x0f)
	o.byte(byte(op))
	o.byte(typeRMISizeCode(t))
	o.mod(ModReg, regRO(r), regRM(r2))
	o.int8(val)
	o.copy(text.Extend(o.len()))
}

// D

type Db byte    // opcode byte
type Dd byte    // opcode byte
type D2d uint16 // two opcode bytes
type D12 uint32 // combination

func (Db) Size() int8  { return 2 }
func (Dd) Size() int8  { return 5 }
func (D2d) Size() int8 { return 6 }

func (ops D12) Addr(text *Buf, addr int32) {
	const (
		insnSize8  = 2
		insnSize32 = 6
	)

	var o output

	if disp := addrDisp(text.Addr, insnSize8, addr); uint32(disp+128) <= 255 {
		o.byte(uint8(ops))
		o.int8(int8(disp))
	} else {
		disp = addrDisp(text.Addr, insnSize32, addr)
		o.word(uint16(ops >> 16))
		o.int32(disp)
	}

	o.copy(text.Extend(o.len()))
}

func (ops D12) AddrStub(text *Buf) {
	const insnSize = 6

	var o output
	o.word(uint16(ops >> 16))
	o.int32(-insnSize) // infinite loop as placeholder
	o.copy(text.Extend(o.len()))
}

func (op Db) Rel8(text *Buf, disp int8) {
	var o output
	o.byte(byte(op))
	o.int8(disp)
	o.copy(text.Extend(o.len()))
}

func (op Db) Addr8(text *Buf, addr int32) {
	const insnSize = 2

	disp := addrDisp(text.Addr, insnSize, addr)

	var o output
	o.byte(byte(op))
	o.int8(int8(disp))
	o.copy(text.Extend(o.len()))
}

func (ops D12) Stub(text *Buf, near bool) {
	const (
		insnSize8  = 2
		insnSize32 = 6
	)

	if near {
		var o output
		o.byte(uint8(ops))
		o.int8(-insnSize8) // infinite loop as placeholder
		o.copy(text.Extend(o.len()))
	} else {
		var o output
		o.word(uint16(ops >> 16))
		o.int32(-insnSize32) // infinite loop as placeholder
		o.copy(text.Extend(o.len()))
	}
}

func (op Db) Stub8(text *Buf) {
	const insnSize = 2

	disp := -insnSize // infinite loop as placeholder

	var o output
	o.byte(byte(op))
	o.int8(int8(disp))
	o.copy(text.Extend(o.len()))
}

func (op Dd) Addr32(text *Buf, addr int32) {
	const insnSize = 5

	disp := addrDisp(text.Addr, insnSize, addr)

	var o output
	o.byte(byte(op))
	o.int32(disp)
	o.copy(text.Extend(o.len()))
}

func (op D2d) Addr32(text *Buf, addr int32) {
	const insnSize = 6

	disp := addrDisp(text.Addr, insnSize, addr)

	var o output
	o.word(uint16(op))
	o.int32(disp)
	o.copy(text.Extend(o.len()))
}

func (op Dd) Stub32(text *Buf) {
	const insnSize = 5

	var o output
	o.byte(byte(op))
	o.int32(-insnSize) // infinite loop as placeholder
	o.copy(text.Extend(o.len()))
}

func (op D2d) Stub32(text *Buf) {
	const insnSize = 6

	var o output
	o.word(uint16(op))
	o.int32(-insnSize) // infinite loop as placeholder
	o.copy(text.Extend(o.len()))
}

func (op Dd) MissingFunction(text *Buf, align bool) {
	const insnSize = 5

	var o output

	if align {
		// Position of disp must be aligned.
		if n := (text.Addr + insnSize - 4) & 3; n > 0 {
			size := 4 - n
			copy(o.buf[:size], nops[size][:size])
			o.offset = uint8(size)
		}
	}

	siteAddr := text.Addr + int32(o.offset) + insnSize
	disp := -siteAddr // NoFunction trap

	o.byte(byte(op))
	o.int32(disp)
	o.copy(text.Extend(o.len()))
}
