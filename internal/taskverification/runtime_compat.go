package taskverification

import "errors"

func execErrorAs(err error, target any) bool {
	return errors.As(err, target)
}
