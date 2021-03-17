// Code generated by command: go run avx512.go -out avx512.s -stubs avx512_stubs.go. DO NOT EDIT.

#include "textflag.h"

// func int8NegAvx512Asm(x []int8, r []int8)
// Requires: AVX512BW, AVX512F
TEXT ·int8NegAvx512Asm(SB), NOSPLIT, $0-48
	MOVQ   x_base+0(FP), AX
	MOVQ   r_base+24(FP), CX
	MOVQ   x_len+8(FP), DX
	VPXORD Z0, Z0, Z0

int8NegBlockLoop:
	CMPQ     DX, $0x00000300
	JL       int8NegTailLoop
	VMOVDQU8 (AX), Z1
	VMOVDQU8 64(AX), Z2
	VMOVDQU8 128(AX), Z3
	VMOVDQU8 192(AX), Z4
	VMOVDQU8 256(AX), Z5
	VMOVDQU8 320(AX), Z6
	VMOVDQU8 384(AX), Z7
	VMOVDQU8 448(AX), Z8
	VMOVDQU8 512(AX), Z9
	VMOVDQU8 576(AX), Z10
	VMOVDQU8 640(AX), Z11
	VMOVDQU8 704(AX), Z12
	VPSUBB   Z0, Z1, Z1
	VPSUBB   Z0, Z2, Z2
	VPSUBB   Z0, Z3, Z3
	VPSUBB   Z0, Z4, Z4
	VPSUBB   Z0, Z5, Z5
	VPSUBB   Z0, Z6, Z6
	VPSUBB   Z0, Z7, Z7
	VPSUBB   Z0, Z8, Z8
	VPSUBB   Z0, Z9, Z9
	VPSUBB   Z0, Z10, Z10
	VPSUBB   Z0, Z11, Z11
	VPSUBB   Z0, Z12, Z12
	VMOVDQU8 Z1, (CX)
	VMOVDQU8 Z2, 64(CX)
	VMOVDQU8 Z3, 128(CX)
	VMOVDQU8 Z4, 192(CX)
	VMOVDQU8 Z5, 256(CX)
	VMOVDQU8 Z6, 320(CX)
	VMOVDQU8 Z7, 384(CX)
	VMOVDQU8 Z8, 448(CX)
	VMOVDQU8 Z9, 512(CX)
	VMOVDQU8 Z10, 576(CX)
	VMOVDQU8 Z11, 640(CX)
	VMOVDQU8 Z12, 704(CX)
	ADDQ     $0x00000300, AX
	ADDQ     $0x00000300, CX
	SUBQ     $0x00000300, DX
	JMP      int8NegBlockLoop

int8NegTailLoop:
	CMPQ     DX, $0x00000040
	JL       int8NegDone
	VMOVDQU8 (AX), Z1
	VPSUBB   Z0, Z1, Z1
	VMOVDQU8 Z1, (CX)
	ADDQ     $0x00000040, AX
	ADDQ     $0x00000040, CX
	SUBQ     $0x00000040, DX
	JMP      int8NegTailLoop

int8NegDone:
	RET

// func int16NegAvx512Asm(x []int16, r []int16)
// Requires: AVX512BW, AVX512F
TEXT ·int16NegAvx512Asm(SB), NOSPLIT, $0-48
	MOVQ   x_base+0(FP), AX
	MOVQ   r_base+24(FP), CX
	MOVQ   x_len+8(FP), DX
	VPXORD Z0, Z0, Z0

int16NegBlockLoop:
	CMPQ      DX, $0x00000180
	JL        int16NegTailLoop
	VMOVDQU16 (AX), Z1
	VMOVDQU16 64(AX), Z2
	VMOVDQU16 128(AX), Z3
	VMOVDQU16 192(AX), Z4
	VMOVDQU16 256(AX), Z5
	VMOVDQU16 320(AX), Z6
	VMOVDQU16 384(AX), Z7
	VMOVDQU16 448(AX), Z8
	VMOVDQU16 512(AX), Z9
	VMOVDQU16 576(AX), Z10
	VMOVDQU16 640(AX), Z11
	VMOVDQU16 704(AX), Z12
	VPSUBW    Z0, Z1, Z1
	VPSUBW    Z0, Z2, Z2
	VPSUBW    Z0, Z3, Z3
	VPSUBW    Z0, Z4, Z4
	VPSUBW    Z0, Z5, Z5
	VPSUBW    Z0, Z6, Z6
	VPSUBW    Z0, Z7, Z7
	VPSUBW    Z0, Z8, Z8
	VPSUBW    Z0, Z9, Z9
	VPSUBW    Z0, Z10, Z10
	VPSUBW    Z0, Z11, Z11
	VPSUBW    Z0, Z12, Z12
	VMOVDQU16 Z1, (CX)
	VMOVDQU16 Z2, 64(CX)
	VMOVDQU16 Z3, 128(CX)
	VMOVDQU16 Z4, 192(CX)
	VMOVDQU16 Z5, 256(CX)
	VMOVDQU16 Z6, 320(CX)
	VMOVDQU16 Z7, 384(CX)
	VMOVDQU16 Z8, 448(CX)
	VMOVDQU16 Z9, 512(CX)
	VMOVDQU16 Z10, 576(CX)
	VMOVDQU16 Z11, 640(CX)
	VMOVDQU16 Z12, 704(CX)
	ADDQ      $0x00000300, AX
	ADDQ      $0x00000300, CX
	SUBQ      $0x00000180, DX
	JMP       int16NegBlockLoop

int16NegTailLoop:
	CMPQ      DX, $0x00000020
	JL        int16NegDone
	VMOVDQU16 (AX), Z1
	VPSUBW    Z0, Z1, Z1
	VMOVDQU16 Z1, (CX)
	ADDQ      $0x00000040, AX
	ADDQ      $0x00000040, CX
	SUBQ      $0x00000020, DX
	JMP       int16NegTailLoop

int16NegDone:
	RET

// func int32NegAvx512Asm(x []int32, r []int32)
// Requires: AVX512F
TEXT ·int32NegAvx512Asm(SB), NOSPLIT, $0-48
	MOVQ   x_base+0(FP), AX
	MOVQ   r_base+24(FP), CX
	MOVQ   x_len+8(FP), DX
	VPXORD Z0, Z0, Z0

int32NegBlockLoop:
	CMPQ      DX, $0x000000c0
	JL        int32NegTailLoop
	VMOVDQU32 (AX), Z1
	VMOVDQU32 64(AX), Z2
	VMOVDQU32 128(AX), Z3
	VMOVDQU32 192(AX), Z4
	VMOVDQU32 256(AX), Z5
	VMOVDQU32 320(AX), Z6
	VMOVDQU32 384(AX), Z7
	VMOVDQU32 448(AX), Z8
	VMOVDQU32 512(AX), Z9
	VMOVDQU32 576(AX), Z10
	VMOVDQU32 640(AX), Z11
	VMOVDQU32 704(AX), Z12
	VPSUBD    Z0, Z1, Z1
	VPSUBD    Z0, Z2, Z2
	VPSUBD    Z0, Z3, Z3
	VPSUBD    Z0, Z4, Z4
	VPSUBD    Z0, Z5, Z5
	VPSUBD    Z0, Z6, Z6
	VPSUBD    Z0, Z7, Z7
	VPSUBD    Z0, Z8, Z8
	VPSUBD    Z0, Z9, Z9
	VPSUBD    Z0, Z10, Z10
	VPSUBD    Z0, Z11, Z11
	VPSUBD    Z0, Z12, Z12
	VMOVDQU32 Z1, (CX)
	VMOVDQU32 Z2, 64(CX)
	VMOVDQU32 Z3, 128(CX)
	VMOVDQU32 Z4, 192(CX)
	VMOVDQU32 Z5, 256(CX)
	VMOVDQU32 Z6, 320(CX)
	VMOVDQU32 Z7, 384(CX)
	VMOVDQU32 Z8, 448(CX)
	VMOVDQU32 Z9, 512(CX)
	VMOVDQU32 Z10, 576(CX)
	VMOVDQU32 Z11, 640(CX)
	VMOVDQU32 Z12, 704(CX)
	ADDQ      $0x00000300, AX
	ADDQ      $0x00000300, CX
	SUBQ      $0x000000c0, DX
	JMP       int32NegBlockLoop

int32NegTailLoop:
	CMPQ      DX, $0x00000010
	JL        int32NegDone
	VMOVDQU32 (AX), Z1
	VPSUBD    Z0, Z1, Z1
	VMOVDQU32 Z1, (CX)
	ADDQ      $0x00000040, AX
	ADDQ      $0x00000040, CX
	SUBQ      $0x00000010, DX
	JMP       int32NegTailLoop

int32NegDone:
	RET

// func int64NegAvx512Asm(x []int64, r []int64)
// Requires: AVX512F
TEXT ·int64NegAvx512Asm(SB), NOSPLIT, $0-48
	MOVQ   x_base+0(FP), AX
	MOVQ   r_base+24(FP), CX
	MOVQ   x_len+8(FP), DX
	VPXORD Z0, Z0, Z0

int64NegBlockLoop:
	CMPQ      DX, $0x00000060
	JL        int64NegTailLoop
	VMOVDQU64 (AX), Z1
	VMOVDQU64 64(AX), Z2
	VMOVDQU64 128(AX), Z3
	VMOVDQU64 192(AX), Z4
	VMOVDQU64 256(AX), Z5
	VMOVDQU64 320(AX), Z6
	VMOVDQU64 384(AX), Z7
	VMOVDQU64 448(AX), Z8
	VMOVDQU64 512(AX), Z9
	VMOVDQU64 576(AX), Z10
	VMOVDQU64 640(AX), Z11
	VMOVDQU64 704(AX), Z12
	VPSUBQ    Z0, Z1, Z1
	VPSUBQ    Z0, Z2, Z2
	VPSUBQ    Z0, Z3, Z3
	VPSUBQ    Z0, Z4, Z4
	VPSUBQ    Z0, Z5, Z5
	VPSUBQ    Z0, Z6, Z6
	VPSUBQ    Z0, Z7, Z7
	VPSUBQ    Z0, Z8, Z8
	VPSUBQ    Z0, Z9, Z9
	VPSUBQ    Z0, Z10, Z10
	VPSUBQ    Z0, Z11, Z11
	VPSUBQ    Z0, Z12, Z12
	VMOVDQU64 Z1, (CX)
	VMOVDQU64 Z2, 64(CX)
	VMOVDQU64 Z3, 128(CX)
	VMOVDQU64 Z4, 192(CX)
	VMOVDQU64 Z5, 256(CX)
	VMOVDQU64 Z6, 320(CX)
	VMOVDQU64 Z7, 384(CX)
	VMOVDQU64 Z8, 448(CX)
	VMOVDQU64 Z9, 512(CX)
	VMOVDQU64 Z10, 576(CX)
	VMOVDQU64 Z11, 640(CX)
	VMOVDQU64 Z12, 704(CX)
	ADDQ      $0x00000300, AX
	ADDQ      $0x00000300, CX
	SUBQ      $0x00000060, DX
	JMP       int64NegBlockLoop

int64NegTailLoop:
	CMPQ      DX, $0x00000008
	JL        int64NegDone
	VMOVDQU64 (AX), Z1
	VPSUBQ    Z0, Z1, Z1
	VMOVDQU64 Z1, (CX)
	ADDQ      $0x00000040, AX
	ADDQ      $0x00000040, CX
	SUBQ      $0x00000008, DX
	JMP       int64NegTailLoop

int64NegDone:
	RET

// func float32NegAvx512Asm(x []float32, r []float32)
// Requires: AVX512F, SSE2
TEXT ·float32NegAvx512Asm(SB), NOSPLIT, $0-48
	MOVQ         x_base+0(FP), AX
	MOVQ         r_base+24(FP), CX
	MOVQ         x_len+8(FP), DX
	MOVL         $0x80000000, BX
	MOVD         BX, X0
	VPBROADCASTD X0, Z0

float32NegBlockLoop:
	CMPQ      DX, $0x000000c0
	JL        float32NegTailLoop
	VMOVDQU32 (AX), Z1
	VMOVDQU32 64(AX), Z2
	VMOVDQU32 128(AX), Z3
	VMOVDQU32 192(AX), Z4
	VMOVDQU32 256(AX), Z5
	VMOVDQU32 320(AX), Z6
	VMOVDQU32 384(AX), Z7
	VMOVDQU32 448(AX), Z8
	VMOVDQU32 512(AX), Z9
	VMOVDQU32 576(AX), Z10
	VMOVDQU32 640(AX), Z11
	VMOVDQU32 704(AX), Z12
	VPXORD    Z0, Z1, Z1
	VPXORD    Z0, Z2, Z2
	VPXORD    Z0, Z3, Z3
	VPXORD    Z0, Z4, Z4
	VPXORD    Z0, Z5, Z5
	VPXORD    Z0, Z6, Z6
	VPXORD    Z0, Z7, Z7
	VPXORD    Z0, Z8, Z8
	VPXORD    Z0, Z9, Z9
	VPXORD    Z0, Z10, Z10
	VPXORD    Z0, Z11, Z11
	VPXORD    Z0, Z12, Z12
	VMOVDQU32 Z1, (CX)
	VMOVDQU32 Z2, 64(CX)
	VMOVDQU32 Z3, 128(CX)
	VMOVDQU32 Z4, 192(CX)
	VMOVDQU32 Z5, 256(CX)
	VMOVDQU32 Z6, 320(CX)
	VMOVDQU32 Z7, 384(CX)
	VMOVDQU32 Z8, 448(CX)
	VMOVDQU32 Z9, 512(CX)
	VMOVDQU32 Z10, 576(CX)
	VMOVDQU32 Z11, 640(CX)
	VMOVDQU32 Z12, 704(CX)
	ADDQ      $0x00000300, AX
	ADDQ      $0x00000300, CX
	SUBQ      $0x000000c0, DX
	JMP       float32NegBlockLoop

float32NegTailLoop:
	CMPQ      DX, $0x00000010
	JL        float32NegDone
	VMOVDQU32 (AX), Z1
	VPXORD    Z0, Z1, Z1
	VMOVDQU32 Z1, (CX)
	ADDQ      $0x00000040, AX
	ADDQ      $0x00000040, CX
	SUBQ      $0x00000010, DX
	JMP       float32NegTailLoop

float32NegDone:
	RET

// func float64NegAvx512Asm(x []float64, r []float64)
// Requires: AVX512F, SSE2
TEXT ·float64NegAvx512Asm(SB), NOSPLIT, $0-48
	MOVQ         x_base+0(FP), AX
	MOVQ         r_base+24(FP), CX
	MOVQ         x_len+8(FP), DX
	MOVQ         $0x8000000000000000, BX
	MOVQ         BX, X0
	VPBROADCASTQ X0, Z0

float64NegBlockLoop:
	CMPQ      DX, $0x00000060
	JL        float64NegTailLoop
	VMOVDQU32 (AX), Z1
	VMOVDQU32 64(AX), Z2
	VMOVDQU32 128(AX), Z3
	VMOVDQU32 192(AX), Z4
	VMOVDQU32 256(AX), Z5
	VMOVDQU32 320(AX), Z6
	VMOVDQU32 384(AX), Z7
	VMOVDQU32 448(AX), Z8
	VMOVDQU32 512(AX), Z9
	VMOVDQU32 576(AX), Z10
	VMOVDQU32 640(AX), Z11
	VMOVDQU32 704(AX), Z12
	VPXORD    Z0, Z1, Z1
	VPXORD    Z0, Z2, Z2
	VPXORD    Z0, Z3, Z3
	VPXORD    Z0, Z4, Z4
	VPXORD    Z0, Z5, Z5
	VPXORD    Z0, Z6, Z6
	VPXORD    Z0, Z7, Z7
	VPXORD    Z0, Z8, Z8
	VPXORD    Z0, Z9, Z9
	VPXORD    Z0, Z10, Z10
	VPXORD    Z0, Z11, Z11
	VPXORD    Z0, Z12, Z12
	VMOVDQU32 Z1, (CX)
	VMOVDQU32 Z2, 64(CX)
	VMOVDQU32 Z3, 128(CX)
	VMOVDQU32 Z4, 192(CX)
	VMOVDQU32 Z5, 256(CX)
	VMOVDQU32 Z6, 320(CX)
	VMOVDQU32 Z7, 384(CX)
	VMOVDQU32 Z8, 448(CX)
	VMOVDQU32 Z9, 512(CX)
	VMOVDQU32 Z10, 576(CX)
	VMOVDQU32 Z11, 640(CX)
	VMOVDQU32 Z12, 704(CX)
	ADDQ      $0x00000300, AX
	ADDQ      $0x00000300, CX
	SUBQ      $0x00000060, DX
	JMP       float64NegBlockLoop

float64NegTailLoop:
	CMPQ      DX, $0x00000008
	JL        float64NegDone
	VMOVDQU32 (AX), Z1
	VPXORD    Z0, Z1, Z1
	VMOVDQU32 Z1, (CX)
	ADDQ      $0x00000040, AX
	ADDQ      $0x00000040, CX
	SUBQ      $0x00000008, DX
	JMP       float64NegTailLoop

float64NegDone:
	RET
