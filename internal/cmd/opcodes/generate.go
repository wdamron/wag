package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	input, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	output, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer output.Close()

	cmd := exec.Command("gofmt")
	cmd.Stdout = output
	cmd.Stderr = os.Stderr

	w, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	if err := generate(w, input); err != nil {
		log.Fatal(err)
	}

	if err := w.Close(); err != nil {
		log.Fatal(err)
	}

	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
	}
}

func generate(w io.Writer, input []byte) (err error) {
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
	}()

	generatePanic(w, string(input))
	return
}

type opcode struct {
	name, sym, imm string
}

func generatePanic(w io.Writer, input string) {
	opcodes := make([]opcode, 256)

	re := regexp.MustCompile("\n\\| +`([^`]+)` +\\| +`(0x[0-9a-f]{2})` +\\| +([a-z_]+ +: +)?`?([^ `|]*)`?[ |]")
	for _, m := range re.FindAllStringSubmatch(input, -1) {
		i, err := strconv.ParseUint(m[2], 0, 8)
		if err != nil {
			panic(err)
		}

		opcodes[i] = opcode{
			name: m[1],
			sym:  symbol(m[1]),
			imm:  symbol(m[4]),
		}
	}

	out := func(format string, args ...interface{}) {
		if _, err := fmt.Fprintf(w, format+"\n", args...); err != nil {
			panic(err)
		}
	}

	out(`package wag`)

	out(`import (`)
	out(`    "github.com/tsavola/wag/internal/loader"`)
	out(`    "github.com/tsavola/wag/internal/opers"`)
	out(`    "github.com/tsavola/wag/types"`)
	out(`)`)

	out(`const (`)
	for code, op := range opcodes {
		if op.name != "" {
			out(`opcode%s = opcode(0x%02x)`, op.sym, code)
		}
	}
	out(`)`)
	out(``)
	out(`var opcodeStrings = [256]string{`)
	for _, op := range opcodes {
		if op.name != "" {
			out(`opcode%s: "%s",`, op.sym, op.name)
		}
	}
	out(`}`)
	out(``)
	out(`var opcodeImpls = [256]opImpl{`)
	for code, op := range opcodes {
		switch op.name {
		case "":
			out(`0x%02x: {badGen, 0},`, code)

		case "block", "loop", "if":
			out(`opcode%s: {nil, 0}, // initialized by init()`, op.sym)

		case "else":
			out(`opcode%s: {badGen, 0},`, op.sym)

		case "end":
			out(`opcode%s: {nil, 0},`, op.sym)

		default:
			if m := regexp.MustCompile("^(...)\\.const$").FindStringSubmatch(op.name); m != nil {
				var (
					impl  = "genConst" + strings.ToUpper(m[1])
					type1 = "types." + strings.ToUpper(m[1])
				)

				out(`opcode%s: {%s, opInfo(%s)},`, op.sym, impl, type1)
			} else if m := regexp.MustCompile("^(...)\\.(.+)/(...)$").FindStringSubmatch(op.name); m != nil {
				var (
					impl  = "genConversionOp"
					type1 = "types." + strings.ToUpper(m[1])
					oper  = "opers." + symbol(m[2])
					type2 = "types." + strings.ToUpper(m[3])
				)

				out(`opcode%s: {%s, opInfo(%s) | (opInfo(%s) << 8) | (opInfo(%s) << 16)},`, op.sym, impl, type1, type2, oper)
			} else if m := regexp.MustCompile("^(.)(..)\\.(load|store)(.*)$").FindStringSubmatch(op.name); m != nil {
				var (
					impl  = "gen" + symbol(m[3]) + "Op"
					type1 = "types." + strings.ToUpper(m[1]+m[2])
					oper  string
				)

				if m[4] == "" {
					oper = "opers." + strings.ToUpper(m[1]+m[2]) + symbol(m[3])
				} else {
					oper = "opers." + typeCategory(m[1]) + symbol(m[3]+m[4])
				}

				out(`opcode%s: {%s, opInfo(%s) | (opInfo(%s) << 16)},`, op.sym, impl, type1, oper)
			} else if m := regexp.MustCompile("^(.)(..)\\.(.+)$").FindStringSubmatch(op.name); m != nil {
				var (
					impl  = operGen(m[3])
					type1 = "types." + strings.ToUpper(m[1]+m[2])
					oper  = "opers." + typeCategory(m[1]) + symbol(m[3])
				)

				out(`opcode%s: {%s, opInfo(%s) | (opInfo(%s) << 16)},`, op.sym, impl, type1, oper)
			} else {
				var (
					impl = "gen" + op.sym
				)

				out(`opcode%s: {%s, 0},`, op.sym, impl)
			}
		}
	}
	out(`}`)
	out(``)
	out(`var opcodeSkips = [256]func(loader.L, opcode){`)
	for code, op := range opcodes {
		switch op.name {
		case "":
			out(`0x%02x: badSkip,`, code)

		case "block", "loop", "if":
			out(`opcode%s: nil, // initialized by init()`, op.sym)

		case "else":
			out(`opcode%s: badSkip,`, op.sym)

		case "end":
			out(`opcode%s: nil,`, op.sym)

		case "br_table", "call_indirect":
			out(`opcode%s: skip%s,`, op.sym, op.sym)

		default:
			if op.imm != "" {
				out(`opcode%s: skip%s,`, op.sym, op.imm)
			} else {
				out(`opcode%s: skipNothing,`, op.sym)
			}
		}
	}
	out(`}`)
}

func symbol(s string) string {
	s = strings.Replace(s, "_", ".", -1)
	s = strings.Title(s)
	s = strings.Replace(s, ".", "", -1)
	s = strings.Replace(s, "/", "", -1)
	return s
}

func typeCategory(letter string) string {
	switch letter {
	case "i":
		return "Int"

	case "f":
		return "Float"
	}

	panic(errors.New(letter))
}

func operGen(oper string) string {
	switch oper {
	case "abs", "ceil", "clz", "copysign", "ctz", "floor", "nearest", "neg", "popcnt", "sqrt", "trunc":
		return "genUnaryOp"

	case "eqz":
		return "genUnaryConditionOp"

	case "add", "and", "max", "min", "mul", "or", "xor":
		return "genBinaryCommuteOp"

	case "div", "div_s", "div_u", "rem_s", "rem_u", "rotl", "rotr", "shl", "shr_s", "shr_u", "sub":
		return "genBinaryOp"

	case "eq", "ne":
		return "genBinaryConditionCommuteOp"

	case "ge", "ge_s", "ge_u", "gt", "gt_s", "gt_u", "le", "le_s", "le_u", "lt", "lt_s", "lt_u":
		return "genBinaryConditionOp"
	}

	panic(errors.New(oper))
}
