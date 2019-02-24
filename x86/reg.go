// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package in

import (
	"fmt"
)

type Reg byte

func (r Reg) String() string {
	return fmt.Sprintf("r%d", r)
}

const (
	Result     = Reg(0)
	ScratchISA = Reg(1) // for internal ISA implementation use
)
