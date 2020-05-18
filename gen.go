//+build generate

//go:generate go run gen.go -out galois_gen_amd64.s -stubs galois_gen_amd64.go
//go:generate gofmt -w galois_gen_switch_amd64.go

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	. "github.com/mmcloughlin/avo/build"
	"github.com/mmcloughlin/avo/buildtags"
	. "github.com/mmcloughlin/avo/operand"
	"github.com/mmcloughlin/avo/reg"
)

// Technically we can do 11x11, but we stay "reasonable".
const inputMax = 12
const outputMax = 10

var switchDefs [inputMax][outputMax]string
var switchDefsX [inputMax][outputMax]string

func main() {
	Constraint(buildtags.Not("appengine").ToConstraint())
	Constraint(buildtags.Not("noasm").ToConstraint())
	Constraint(buildtags.Term("gc").ToConstraint())

	for i := 1; i <= inputMax; i++ {
		for j := 1; j <= outputMax; j++ {
			//genMulAvx2(fmt.Sprintf("mulAvxTwoXor_%dx%d", i, j), i, j, true)
			genMulAvx2(fmt.Sprintf("mulAvxTwo_%dx%d", i, j), i, j, false)
		}
	}
	f, err := os.Create("galois_gen_switch_amd64.go")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	w.WriteString(`// Code generated by command: ` + strings.Join(os.Args, " ") + ` DO NOT EDIT.

// +build !appengine
// +build !noasm
// +build gc

package reedsolomon

import "fmt"

`)

	w.WriteString(fmt.Sprintf("const maxAvx2Inputs = %d\nconst maxAvx2Outputs = %d\n", inputMax, outputMax))

	w.WriteString(`

func galMulSlicesAvx2(matrixRows [][]byte, in, out [][]byte, start, stop int) {
	switch len(in) {
`)
	for in, defs := range switchDefs[:] {
		w.WriteString(fmt.Sprintf("		case %d:\n			switch len(out) {\n", in+1))
		for out, def := range defs[:] {
			w.WriteString(fmt.Sprintf("				case %d:\n", out+1))
			w.WriteString(def)
		}
		w.WriteString("}\n")
	}
	w.WriteString(`}
	panic(fmt.Sprintf("unhandled size: %dx%d", len(in), len(out)))
}
`)
	Generate()
}

// [6][16]byte, high [6][16]byte, in [3][]byte, out [2][]byte -> 15

func genMulAvx2(name string, inputs int, outputs int, xor bool) {
	total := inputs * outputs
	const perLoopBits = 5
	const perLoop = 1 << perLoopBits

	doc := []string{
		fmt.Sprintf("%s takes %d inputs and produces %d outputs.", name, inputs, outputs),
	}
	if !xor {
		doc = append(doc, "The output is initialized to 0.")
	}

	// Load shuffle masks on every use.
	var loadNone bool
	// Use registers for destination registers.
	var regDst = true

	// lo, hi, 1 in, 1 out, 2 tmp, 1 mask
	est := total*2 + outputs + 5
	if outputs == 1 {
		// We don't need to keep a copy of the input if only 1 output.
		est -= 2
	}

	if est > 16 {
		if est > 16 {
			loadNone = true
			// We run out of GP registers first, now.
			if inputs+outputs > 13 {
				regDst = false
			}
		} else {
			//Comment("Loading half tables to registers")
		}
	}

	if loadNone {
		TEXT(name, 0, fmt.Sprintf("func(low, high [%d][16]byte, in [%d][]byte, out [%d][]byte)", total*2, inputs, outputs))
		Comment("Loading no tables to registers")

		// SWITCH DEFINITION:
		s := ""
		s += fmt.Sprintf("			mulAvxTwo_%dx%d([%d][16]byte{\n", inputs, outputs, total*2)
		for out := 0; out < outputs; out++ {
			for in := 0; in < inputs; in++ {
				s += fmt.Sprintf("\t\t\t\tmulTableLow[matrixRows[%d][%d]], mulTableLow[matrixRows[%d][%d]],\n", out, in, out, in)
			}
		}
		s += fmt.Sprintf("			}, [%d][16]byte{", total*2)
		for out := 0; out < outputs; out++ {
			for in := 0; in < inputs; in++ {
				s += fmt.Sprintf("\t\t\t\tmulTableHigh[matrixRows[%d][%d]], mulTableHigh[matrixRows[%d][%d]],\n", out, in, out, in)
			}
		}
		s += fmt.Sprintf(`				},
					[%d][]byte{`, inputs)
		for in := 0; in < inputs; in++ {
			s += fmt.Sprintf("in[%d][start:stop],", in)
		}
		s += fmt.Sprintf(`
					},
					[%d][]byte{`, outputs)
		for out := 0; out < outputs; out++ {
			s += fmt.Sprintf("out[%d][start:stop],", out)
		}
		s += `
				},
			)
			return
`

		switchDefs[inputs-1][outputs-1] = s
	} else {
		TEXT(name, 0, fmt.Sprintf("func(low, high [%d][16]byte, in [%d][]byte, out [%d][]byte)", total, inputs, outputs))
		Comment("Loading all tables to registers")

		// SWITCH DEFINITION:
		s := ""
		s += fmt.Sprintf("			mulAvxTwo_%dx%d([%d][16]byte{\n", inputs, outputs, total)
		for out := 0; out < outputs; out++ {
			for in := 0; in < inputs; in++ {
				s += fmt.Sprintf("\t\t\t\tmulTableLow[matrixRows[%d][%d]],\n", out, in)
			}
		}
		s += fmt.Sprintf("			}, [%d][16]byte{", total)
		for out := 0; out < outputs; out++ {
			for in := 0; in < inputs; in++ {
				s += fmt.Sprintf("\t\t\t\tmulTableHigh[matrixRows[%d][%d]],\n", out, in)
			}
		}
		s += fmt.Sprintf(`				},
					[%d][]byte{`, inputs)
		for in := 0; in < inputs; in++ {
			s += fmt.Sprintf("in[%d][start:stop],", in)
		}
		s += fmt.Sprintf(`
					},
					[%d][]byte{`, outputs)
		for out := 0; out < outputs; out++ {
			s += fmt.Sprintf("out[%d][start:stop],", out)
		}
		s += `
				},
			)
			return
`
		switchDefs[inputs-1][outputs-1] = s
	}

	Doc(doc...)
	Pragma("noescape")

	Commentf("Full registers estimated %d YMM used", est)
	Comment("Load all tables to registers")

	length := Load(Param("in").Index(0).Len(), GP64())
	SHRQ(U8(perLoopBits), length)
	TESTQ(length, length)
	JZ(LabelRef(name + "_end"))

	dst := make([]reg.VecVirtual, outputs)
	dstPtr := make([]reg.GPVirtual, outputs)
	for i := range dst {
		dst[i] = YMM()
		if !regDst {
			continue
		}
		ptr := GP64()
		p, err := Param("out").Index(i).Base().Resolve()
		if err != nil {
			panic(err)
		}
		MOVQ(p.Addr, ptr)
		dstPtr[i] = ptr
	}

	inLo := make([]reg.VecVirtual, total)
	inHi := make([]reg.VecVirtual, total)

	for i := range inLo {
		if loadNone {
			break
		}
		tableLo := YMM()
		MOVOU(Param("low").Index(i).MustAddr(), tableLo.AsX())
		tableHi := YMM()
		MOVOU(Param("high").Index(i).MustAddr(), tableHi.AsX())
		VINSERTI128(U8(1), tableLo.AsX(), tableLo, tableLo)
		VINSERTI128(U8(1), tableHi.AsX(), tableHi, tableHi)
		inLo[i] = tableLo
		inHi[i] = tableHi
	}

	inPtr := make([]reg.GPVirtual, inputs)
	for i := range inPtr {
		ptr := GP64()
		p, err := Param("in").Index(i).Base().Resolve()
		if err != nil {
			panic(err)
		}
		MOVQ(p.Addr, ptr)
		inPtr[i] = ptr
	}

	tmpMask := GP64()
	MOVQ(U32(15), tmpMask)
	lowMask := YMM()
	MOVQ(tmpMask, lowMask.AsX())
	VPBROADCASTB(lowMask.AsX(), lowMask)

	offset := GP64()
	XORQ(offset, offset)
	Label(name + "_loop")
	if xor {
		Commentf("Load %d outputs", outputs)
	} else {
		Commentf("Clear %d outputs", outputs)
	}
	for i := range dst {
		if xor {
			if regDst {
				VMOVDQU(Mem{Base: dstPtr[i], Index: offset, Scale: 1}, dst[i])
				continue
			}
			ptr := GP64()
			p, err := Param("out").Index(i).Base().Resolve()
			if err != nil {
				panic(err)
			}
			MOVQ(p.Addr, ptr)
			VMOVDQU(Mem{Base: ptr, Index: offset, Scale: 1}, dst[i])
		} else {
			VPXOR(dst[i], dst[i], dst[i])
		}
	}

	lookLow, lookHigh := YMM(), YMM()
	inLow, inHigh := YMM(), YMM()
	for i := range inPtr {
		Commentf("Load and process 32 bytes from input %d to %d outputs", i, outputs)
		VMOVDQU(Mem{Base: inPtr[i], Index: offset, Scale: 1}, inLow)
		VPSRLQ(U8(4), inLow, inHigh)
		VPAND(lowMask, inLow, inLow)
		VPAND(lowMask, inHigh, inHigh)
		for j := range dst {
			if loadNone {
				VMOVDQU(Param("low").Index(2*(i*outputs+j)).MustAddr(), lookLow.AsY())
				VMOVDQU(Param("high").Index(2*(i*outputs+j)).MustAddr(), lookHigh.AsY())
				VPSHUFB(inLow, lookLow, lookLow)
				VPSHUFB(inHigh, lookHigh, lookHigh)
			} else {
				VPSHUFB(inLow, inLo[i*outputs+j], lookLow)
				VPSHUFB(inHigh, inHi[i*outputs+j], lookHigh)
			}
			VPXOR(lookLow, lookHigh, lookLow)
			VPXOR(lookLow, dst[j], dst[j])
		}
	}
	Commentf("Store %d outputs", outputs)
	for i := range dst {
		if regDst {
			VMOVDQU(dst[i], Mem{Base: dstPtr[i], Index: offset, Scale: 1})
			continue
		}
		ptr := GP64()
		p, err := Param("out").Index(i).Base().Resolve()
		if err != nil {
			panic(err)
		}
		MOVQ(p.Addr, ptr)
		VMOVDQU(dst[i], Mem{Base: ptr, Index: offset, Scale: 1})
	}
	Comment("Prepare for next loop")
	ADDQ(U8(perLoop), offset)
	DECQ(length)
	JNZ(LabelRef(name + "_loop"))
	VZEROUPPER()

	Label(name + "_end")
	RET()
}
