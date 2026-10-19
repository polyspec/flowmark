package validate

import (
	"testing"

	"github.com/polyspec/orm/packages/go/engine/dbspec"

	"github.com/polyspec/flowmark/packages/go/catalog"
)

// measureCheck returns the table measures of the test catalog and the predicate
// of its check name.
func measureCheck(t *testing.T, name string) (*catalog.Table, dbspec.Expr) {
	t.Helper()
	tb, ok := loadCatalog(t).Table("measures")
	if !ok {
		t.Fatal("the test catalog has no table measures")
	}
	for _, k := range tb.Checks {
		if k.Name == name {
			return tb, k.Expression
		}
	}
	t.Fatalf("table measures has no check %s", name)
	return nil, nil
}

// raw is an operand not of its column type, written as text.
func raw(text string) Operand { return Operand{Text: text, Raw: true} }

// val is an operand in its canonical readable form.
func val(text string) Operand { return Operand{Text: text} }

// operandsOf returns the operands of a row whose columns are m; a column not in
// m is NULL.
func operandsOf(m map[string]Operand) func(string) Operand {
	return func(col string) Operand {
		if o, ok := m[col]; ok {
			return o
		}
		return Operand{Null: true}
	}
}

// TestCatalogChecks pins that the catalog carries each declared check with its
// typed predicate, in declaration order.
func TestCatalogChecks(t *testing.T) {
	tb, ok := loadCatalog(t).Table("measures")
	if !ok {
		t.Fatal("no table measures")
	}
	var names []string
	for _, k := range tb.Checks {
		if k.Expression == nil {
			t.Fatalf("check %s has no predicate", k.Name)
		}
		names = append(names, k.Name)
	}
	want := []string{"ck_measures_range", "ck_measures_positive", "ck_measures_price", "ck_measures_ratio", "ck_measures_span", "ck_measures_label", "ck_measures_code"}
	if len(names) != len(want) {
		t.Fatalf("checks = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("checks = %v, want %v", names, want)
		}
	}
}

// TestEvalCheck holds the accepting and rejecting cases of the evaluator in SQL's
// three-valued logic, with the raw atoms taking the truth value given.
func TestEvalCheck(t *testing.T) {
	cases := []struct {
		name, check string
		values      map[string]Operand
		raw         Truth
		want        Truth
	}{
		{name: "50#checks: integers compare by value, 9 <= 10", check: "ck_measures_range",
			values: map[string]Operand{"low": val("9"), "high": val("10")}, want: TruthTrue},
		{name: "50#checks: integers compare by value, 10 <= 9 is false", check: "ck_measures_range",
			values: map[string]Operand{"low": val("10"), "high": val("9")}, want: TruthFalse},
		{name: "50#checks: a NULL operand makes a comparison unknown", check: "ck_measures_range",
			values: map[string]Operand{"high": val("9")}, want: TruthUnknown},
		{name: "50#checks: a NULL beside a raw value is unknown, not raw", check: "ck_measures_range",
			values: map[string]Operand{"low": raw("x")}, raw: TruthFalse, want: TruthUnknown},
		{name: "50#checks: decimals compare by value, 10.00 >= 2.00", check: "ck_measures_price",
			values: map[string]Operand{"price": val("10.00")}, want: TruthTrue},
		{name: "50#checks: decimals compare by value, 1.99 >= 2.00 is false", check: "ck_measures_price",
			values: map[string]Operand{"price": val("1.99")}, want: TruthFalse},
		{name: "50#checks: f64 compares by value, 10 < 1.5 is false", check: "ck_measures_ratio",
			values: map[string]Operand{"ratio": val("10")}, want: TruthFalse},
		{name: "50#checks: f64 compares by value, 0.5 < 1.5", check: "ck_measures_ratio",
			values: map[string]Operand{"ratio": val("0.5")}, want: TruthTrue},
		{name: "50#checks: datetimes compare chronologically", check: "ck_measures_span",
			values: map[string]Operand{"starts": val("2026-10-01 23:59:59"), "ends": val("2026-10-02 00:00:00")}, want: TruthTrue},
		{name: "50#checks: a later start is false", check: "ck_measures_span",
			values: map[string]Operand{"starts": val("2026-10-02 00:00:00"), "ends": val("2026-10-01 23:59:59")}, want: TruthFalse},
		{name: "50#checks: in holds a listed value", check: "ck_measures_label",
			values: map[string]Operand{"label": val("B")}, want: TruthTrue},
		{name: "50#checks: in is false for an unlisted value", check: "ck_measures_label",
			values: map[string]Operand{"label": val("c")}, want: TruthFalse},
		{name: "50#checks: is null holds for NULL", check: "ck_measures_label",
			values: map[string]Operand{}, want: TruthTrue},
		{name: "50#checks: text compares by code point, so B is below a", check: "ck_measures_code",
			values: map[string]Operand{"code": val("B")}, want: TruthFalse},
		{name: "50#checks: text compares by code point, so b is not below a", check: "ck_measures_code",
			values: map[string]Operand{"code": val("b")}, want: TruthTrue},
		{name: "50#checks: a raw in-atom taken true", check: "ck_measures_label",
			values: map[string]Operand{"label": raw("toolong")}, raw: TruthTrue, want: TruthTrue},
		{name: "50#checks: a raw in-atom taken false, and a raw value is not NULL", check: "ck_measures_label",
			values: map[string]Operand{"label": raw("toolong")}, raw: TruthFalse, want: TruthFalse},
		{name: "50#checks: a raw atom taken true cannot save a false conjunct", check: "ck_measures_positive",
			values: map[string]Operand{"low": raw("x"), "high": val("-1")}, raw: TruthTrue, want: TruthFalse},
		{name: "50#checks: a raw atom taken true", check: "ck_measures_positive",
			values: map[string]Operand{"low": raw("x"), "high": val("1")}, raw: TruthTrue, want: TruthTrue},
		{name: "50#checks: a raw atom taken false", check: "ck_measures_positive",
			values: map[string]Operand{"low": raw("x"), "high": val("1")}, raw: TruthFalse, want: TruthFalse},
		{name: "50#checks: a raw operand of a column comparison", check: "ck_measures_range",
			values: map[string]Operand{"low": val("1"), "high": raw("x")}, raw: TruthFalse, want: TruthFalse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tb, e := measureCheck(t, tc.check)
			if got := EvalCheck(tb, e, operandsOf(tc.values), tc.raw); got != tc.want {
				t.Fatalf("EvalCheck = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestJudgeCheck holds the commit's three verdicts: the check fails when it is
// false with every raw atom true; the type of a raw value fails it when the check
// is not false so but false with every raw atom false; otherwise it passes.
func TestJudgeCheck(t *testing.T) {
	cases := []struct {
		name, check string
		values      map[string]Operand
		want        CheckVerdict
	}{
		{name: "50#checks: a check that holds passes", check: "ck_measures_range",
			values: map[string]Operand{"low": val("1"), "high": val("2")}, want: CheckPasses},
		{name: "50#checks: an unknown check passes", check: "ck_measures_range",
			values: map[string]Operand{"low": val("1")}, want: CheckPasses},
		{name: "50#checks: a false check fails", check: "ck_measures_range",
			values: map[string]Operand{"low": val("3"), "high": val("2")}, want: CheckFails},
		{name: "50#checks: a check false whatever the raw value is fails as the check", check: "ck_measures_positive",
			values: map[string]Operand{"low": raw("x"), "high": val("-1")}, want: CheckFails},
		{name: "50#checks: a check that the raw value decides fails as its type", check: "ck_measures_positive",
			values: map[string]Operand{"low": raw("x"), "high": val("1")}, want: CheckNeedsType},
		{name: "50#checks: a raw in-atom that decides the check fails as its type", check: "ck_measures_label",
			values: map[string]Operand{"label": raw("toolong")}, want: CheckNeedsType},
		{name: "50#checks: a raw value beside a NULL leaves the check unknown, so it passes", check: "ck_measures_range",
			values: map[string]Operand{"low": raw("x")}, want: CheckPasses},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tb, e := measureCheck(t, tc.check)
			if got := JudgeCheck(tb, e, operandsOf(tc.values)); got != tc.want {
				t.Fatalf("JudgeCheck = %v, want %v", got, tc.want)
			}
		})
	}
}
