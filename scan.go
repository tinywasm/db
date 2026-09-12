package storage

import . "webtyp.com/fmt"

// ScanAny maps a driver value into a typed pointer. It is the single
// definition of what a database value means in Go: a nil (NULL) scans as
// the Go zero value, in every backend.
func ScanAny(v any, dest any) error {
	if v == nil {
		switch p := dest.(type) {
		case *string:
			*p = ""
		case *int:
			*p = 0
		case *int64:
			*p = 0
		case *float64:
			*p = 0
		case *bool:
			*p = false
		case *[]byte:
			*p = nil
		case *any:
			*p = nil
		default:
			return Errf("storage: unsupported scan type: %T", dest)
		}
		return nil
	}
	switch p := dest.(type) {
	case *string:
		switch x := v.(type) {
		case string:
			*p = x
		case []byte:
			*p = string(x)
		case int:
			*p = Convert(x).String()
		case int64:
			*p = Convert(x).String()
		case float64:
			*p = Convert(x).String()
		case bool:
			*p = Convert(x).String()
		default:
			return Errf("storage: cannot scan %T into %T", v, dest)
		}
	case *int:
		switch x := v.(type) {
		case int:
			*p = x
		case int64:
			*p = int(x)
		case float64:
			*p = int(x)
		case bool:
			if x {
				*p = 1
			} else {
				*p = 0
			}
		case []byte:
			n, err := Convert(string(x)).Int()
			if err != nil {
				return Errf("storage: cannot scan %T into %T", v, dest)
			}
			*p = n
		case string:
			n, err := Convert(x).Int()
			if err != nil {
				return Errf("storage: cannot scan %T into %T", v, dest)
			}
			*p = n
		default:
			return Errf("storage: cannot scan %T into %T", v, dest)
		}
	case *int64:
		switch x := v.(type) {
		case int64:
			*p = x
		case int:
			*p = int64(x)
		case float64:
			*p = int64(x)
		case bool:
			if x {
				*p = 1
			} else {
				*p = 0
			}
		case []byte:
			n, err := Convert(string(x)).Int64()
			if err != nil {
				return Errf("storage: cannot scan %T into %T", v, dest)
			}
			*p = n
		case string:
			n, err := Convert(x).Int64()
			if err != nil {
				return Errf("storage: cannot scan %T into %T", v, dest)
			}
			*p = n
		default:
			return Errf("storage: cannot scan %T into %T", v, dest)
		}
	case *float64:
		switch x := v.(type) {
		case float64:
			*p = x
		case int:
			*p = float64(x)
		case int64:
			*p = float64(x)
		case bool:
			if x {
				*p = 1
			} else {
				*p = 0
			}
		case []byte:
			f, err := Convert(string(x)).Float64()
			if err != nil {
				return Errf("storage: cannot scan %T into %T", v, dest)
			}
			*p = f
		case string:
			f, err := Convert(x).Float64()
			if err != nil {
				return Errf("storage: cannot scan %T into %T", v, dest)
			}
			*p = f
		default:
			return Errf("storage: cannot scan %T into %T", v, dest)
		}
	case *bool:
		switch x := v.(type) {
		case bool:
			*p = x
		case int:
			*p = x != 0
		case int64:
			*p = x != 0
		case float64:
			*p = x != 0
		case []byte:
			b, err := Convert(string(x)).Bool()
			if err != nil {
				return Errf("storage: cannot scan %T into %T", v, dest)
			}
			*p = b
		case string:
			b, err := Convert(x).Bool()
			if err != nil {
				return Errf("storage: cannot scan %T into %T", v, dest)
			}
			*p = b
		default:
			return Errf("storage: cannot scan %T into %T", v, dest)
		}
	case *[]byte:
		switch b := v.(type) {
		case []byte:
			*p = b
		case string:
			*p = []byte(b)
		default:
			return Errf("storage: cannot scan %T into %T", v, dest)
		}
	case *any:
		*p = v
	default:
		return Errf("storage: unsupported scan type: %T", dest)
	}
	return nil
}

// NullSafe wraps scan destinations so a NULL column yields the Go zero value
// instead of a driver error. A backend built on the std sql package applies
// it to the destinations it receives, in its own Scan:
//
//	func (r *rows) Scan(dest ...any) error {
//	    return r.rows.Scan(NullSafe(dest)...)
//	}
//
// Each wrapper's Scan method satisfies the std Scanner contract structurally,
// so the driver hands it the raw value and this package — not the driver —
// decides what NULL means.
func NullSafe(dest []any) []any {
	out := make([]any, len(dest))
	for i, d := range dest {
		out[i] = nullDest{dest: d}
	}
	return out
}

// nullDest carries one destination. It is a value, not a pointer: it holds the
// caller's pointer, so copying it copies the reference, not the field.
type nullDest struct{ dest any }

func (n nullDest) Scan(src any) error { return ScanAny(src, n.dest) }
