package imagedefinition

import "errors"

var (
	ErrKeepEnabledNil   = errors.New("KeepEnabled is nil. Thi value cannot be properly used.")
	ErrFstabTruncateNil = errors.New("FstabTruncate is nil. Thi value cannot be properly used.")
)
