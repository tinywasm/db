---
PLAN: "fix: a NULL column scans as the Go zero value, in every backend"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
>
> **Phase A (GATE)** of
> [`NULLABLE_COLUMNS_MASTER_PLAN.md`](https://github.com/webtyp/docs/blob/main/NULLABLE_COLUMNS_MASTER_PLAN.md).
> `webtyp/sqlite` (phase B) and `webtyp/postgres` (phase C) cannot start before
> this ships a tag.

# Plan — `webtyp.com/storage`: NULL is the Go zero value, not an error and not the previous row

## 0. Context (verified against the repo — do not re-diagnose)

This package owns the `Scanner`/`Rows` contract, the only value-assignment
helper (`ScanAny`, in `scan.go`) and the `conformance` suite every backend must
pass. Today the ecosystem answers "what is a NULL column in Go?" **twice, and
differently**:

- `storage/mem` routes through `ScanAny`. Every type assertion fails for a nil
  value, so the destination is **left untouched**. On a fresh model that happens
  to look right (it is already zero); on a `ReadOne` that reuses a model it
  returns **the previous row's value** — wrong data, silently.
- `webtyp/sqlite` and `webtyp/postgres` hand the raw `*string` to
  `database/sql`, which refuses:
  `sql: Scan error on column index 1, name "email": converting NULL to string is unsupported`

So a consumer's unit tests (on `mem`) pass and the same code fails in
production. `conformance` cannot catch it because its `Widget` model declares
every field `NotNull: true` — no backend is ever tested against a nullable
column.

The rule this plan establishes, once, here:

```
A NULL column scans as the Go ZERO VALUE, in every backend.
```

**Anti-footgun.** This package depends only on `webtyp.com/fmt` and
`webtyp.com/model`, and must stay that way: do **NOT** import `database/sql`,
`reflect` or any other stdlib to solve this. It is not needed — a type whose
method set includes `Scan(src any) error` satisfies `database/sql`'s
`sql.Scanner` **structurally**, with no import on our side. That is the whole
mechanism phases B and C rely on.

## Design gate (api-design — five answers)

### 1. Prior art

| Concern | Frameworks | Why we differ |
|---|---|---|
| NULL → Go value | **`database/sql`** (`sql.NullString`, `sql.Null[T]` — the caller declares nullability per field and unwraps `.Valid` by hand) | A per-field wrapper type leaks into every model struct and forces `ormc` to generate two shapes. We keep the model a plain struct and make NULL a property of the *contract*, not of the field's Go type. |
| NULL → Go value | **GORM** (pointer fields `*string`, or `sql.Null*`), **sqlx** (same, plus `MapScan` to `any`) | Both make the application carry nullability in its types. Our models are also the wire and form schema (`model.Definition`), so a `*string` would change JSON, DDL and widget generation for a storage-only concern. |
| NULL → Go value | **Ent**, **Bun** (codegen emits optional fields / `sql.Null*` under the hood) | Same codegen cost, and both still leave "what does a NULL mean" answered by the driver. We answer it once in the contract so every backend agrees by construction. |

Nobody makes NULL a contract-level constant rule, because most ORMs must
support *distinguishing* NULL from zero. We deliberately do not: in this
ecosystem `NotNull` already declares intent at the model, and a nullable column
whose zero value is meaningful is a modelling error, not a case to encode.

### 2. Novice-name test

- `storage.NullSafe(dest)` — "make these scan destinations null-safe". Read at a
  backend's call site: `s.Scan(storage.NullSafe(dest)...)`.
- `storage.ScanAny(v, dest)` — unchanged name, already the package's word for
  "assign this driver value into that pointer".

### 3. Complexity ledger

```
Concepts the developer must learn   +1 (NullSafe, only for backend authors) / −1 (nobody learns "mem and sqlite disagree")
Files they must touch to do X       +0 / −0
Lines at the call site              +0 for consumers; +1 per backend
Ways to do the same thing           +0 / −1 (one definition of "NULL in Go", was two)
```

### 4. Where it belongs

`storage` owns `Scanner`/`Rows`, `ScanAny` and `conformance`. Putting the rule
in `orm` would leave the contract itself untrue for anyone calling
`conn.QueryRow(...).Scan(...)` directly — including `conformance`, which does
exactly that. No second concern enters the package: this IS the scan concern.

### 5. What it deletes

- The silent "leave the destination untouched" branch of `ScanAny` — replaced by
  an explicit zero write.
- The all-`NotNull` `Widget`, which made the defect untestable.

## Quality rules (apply to every stage)

```
RULE: no stdlib imports in this package — webtyp/fmt and webtyp/model only.
      Do NOT import database/sql, reflect, strconv or errors to solve this.
RULE: every repeated string is a named constant; string literals forbidden in logic.
RULE: no silent fallback — a value that cannot be assigned returns an error,
      it never leaves the destination holding a previous row's data.
```

## Stage 1 — `ScanAny` handles the full driver value set and writes zero on nil

**File:** `scan.go`.

Today `ScanAny` only assigns when the dynamic type matches exactly, and does
nothing otherwise — including for `nil`. Both halves are defects: phases B and C
will route **every** value through it (not just NULLs), so it must cover
everything `database/sql` used to convert.

`database/sql` delivers exactly these dynamic types to a `Scanner`:
`int64`, `float64`, `bool`, `[]byte`, `string`, `time.Time`, `nil`.

1. Handle `nil` **first**, before the type switch, writing the zero value:
   ```go
   if v == nil {
       switch p := dest.(type) {
       case *string:  *p = ""
       case *int:     *p = 0
       case *int64:   *p = 0
       case *float64: *p = 0
       case *bool:    *p = false
       case *[]byte:  *p = nil
       case *any:     *p = nil
       default:
           return Errf("storage: unsupported scan type: %T", dest)
       }
       return nil
   }
   ```
2. Extend each non-nil case so the conversions `database/sql` used to perform
   still happen. Required additions:
   - `*string` ← `[]byte` (SQLite returns TEXT as `[]byte`), `int64`, `float64`, `bool`
   - `*int` / `*int64` ← `[]byte`, `string` (numeric text)
   - `*float64` ← `int64`, `[]byte`, `string`
   - `*bool` ← `int64` (0/1), `[]byte`, `string`
   - `*[]byte` ← already handles `string` and `[]byte`; add `nil` via step 1
   Use `webtyp/fmt` for every conversion (`fmt.Convert(x).Int()`,
   `fmt.Convert(x).String()`) — **never** `strconv`.
3. A value that reaches a case and cannot be converted returns
   `Errf("storage: cannot scan %T into %T", v, dest)` — it must NOT fall through
   leaving the destination untouched.

## Stage 2 — `NullSafe`, the adapter backends hand to `database/sql`

**File:** `scan.go`.

```go
// NullSafe wraps scan destinations so a NULL column yields the Go zero value
// instead of a driver error. A backend built on database/sql applies it to the
// destinations it receives, in its own Scan:
//
//	func (r *rows) Scan(dest ...any) error {
//	    return r.rows.Scan(NullSafe(dest)...)
//	}
//
// Each wrapper's Scan method satisfies database/sql's sql.Scanner structurally,
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
```

Do **not** export `nullDest`. `NullSafe` is the entire public surface.

## Stage 3 — conformance proves the rule on every backend

**Files:** `conformance/model.go`, `conformance/conformance.go`.

1. `conformance/model.go`: add a nullable column to `Widget` — the model's
   existing fields all declare `NotNull: true`, which is exactly why this
   defect survived:
   ```go
   {Name: "note", Type: model.Text()}, // nullable ON PURPOSE: proves NULL scans as ""
   ```
   Add the matching `Note string` field, and include it in `Pointers()`,
   `EncodeFields`/`DecodeFields` in the same order as `Schema()`.
2. `conformance/conformance.go`: add a case named `NullScansAsZero` that
   - inserts a widget writing every column EXCEPT `note` (so the DB stores NULL —
   build the `storage.Query` with an explicit `Columns` slice omitting it),
   - reads it back with `readOne` **into a model whose `Note` was pre-set to a
   non-empty sentinel**, so a backend that "leaves the destination untouched"
   fails instead of accidentally passing,
   - asserts `got.Note == ""`.
   Verbatim failure message: `` `NullScansAsZero: note = %q, want "" (a NULL column must scan as the Go zero value)` ``
3. Wire the new case into the exported suite the backends call, next to the
   existing ones.

## Stage 4 — `mem` keeps routing through the fixed helper

**File:** `mem/mem.go`.

`scanInto` already calls `storage.ScanAny`, so it inherits Stage 1. One change
is still required: it skips the assignment entirely when the column is absent
from the row (`if v, ok := row.get(f.Name); ok`). A column that exists with a
nil value must reach `ScanAny`; a column genuinely absent from the row must
**also** write the zero value, for the same reason. Drop the `ok` guard and pass
`v` (nil when absent) straight to `ScanAny`.

## Acceptance criteria

1. `go build ./...`, `go vet ./...`, `go test ./...` green.
2. `grep -rn "database/sql\|reflect\|strconv" --include='*.go' .` → empty
   (tests included; this package stays stdlib-free).
3. `grep -rn "NotNull: true" conformance/` → no longer covers every field of
   `Widget`; the `note` column has no `NotNull`.
4. The `NullScansAsZero` case fails if `ScanAny`'s nil branch is removed —
   verify by temporarily deleting it before finishing.
5. `grep -rn "TODO\|FIXME\|Deprecated" --include='*.go' .` → only hits that
   predate this change.

## Out of scope

- `webtyp/sqlite` and `webtyp/postgres` applying `NullSafe` — phases B and C.
- `orm.Create` honouring `OmitEmpty` — phase D, independent of this one.
- Distinguishing NULL from the zero value. This ecosystem deliberately does not:
  `NotNull` declares intent at the model, and a nullable column whose zero value
  carries meaning is a modelling error.

| Stage | Files | Action |
|---|---|---|
| 1 | `scan.go` | nil → explicit zero; full driver value set; error instead of silent no-op |
| 2 | `scan.go` | `NullSafe` + unexported `nullDest` |
| 3 | `conformance/model.go`, `conformance/conformance.go` | nullable column + `NullScansAsZero` case |
| 4 | `mem/mem.go` | absent/nil column reaches `ScanAny` |
