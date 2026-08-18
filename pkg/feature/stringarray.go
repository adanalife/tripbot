package feature

import (
	"database/sql/driver"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// pgTypes owns the text[] wire format in both directions. Stateless after
// construction and safe for concurrent use.
var pgTypes = pgtype.NewMap()

// stringArray maps a Postgres text[] column onto a []string. Postgres array
// literals quote and escape their elements ({dana,"some one","wi\"th"}), and
// pgx already implements those rules exactly, so both directions hand the
// bytes to its codec rather than re-deriving them here.
//
// A plain []string field does not work: GORM writes any slice it doesn't
// recognise as a record tuple — ('a','b') — which Postgres rejects against a
// text[] column. Satisfying driver.Valuer is what makes GORM treat the field
// as one value instead.
type stringArray []string

// Scan decodes the array literal the pgx stdlib driver reports for a text[]
// column. A NULL column yields a nil slice.
func (a *stringArray) Scan(src any) error {
	var raw []byte
	switch v := src.(type) {
	case nil:
		*a = nil
		return nil
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("feature: cannot scan %T into stringArray", src)
	}
	return pgTypes.Scan(pgtype.TextArrayOID, pgtype.TextFormatCode, raw, (*[]string)(a))
}

// Value encodes back to an array literal. A nil slice becomes SQL NULL, which
// the allowlist columns reject — they are NOT NULL DEFAULT '{}' — so a caller
// meaning "nobody" must pass an empty stringArray, not a nil one.
func (a stringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	buf, err := pgTypes.Encode(pgtype.TextArrayOID, pgtype.TextFormatCode, []string(a), nil)
	if err != nil {
		return nil, err
	}
	return string(buf), nil
}
