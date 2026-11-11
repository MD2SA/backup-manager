package pgcompat

import (
	"io"
	"strings"
	"testing"
)

func TestSQLFilterFiltersPreambleOnly(t *testing.T) {
	input := `SET statement_timeout = 0;
SET transaction_timeout = 0;
COPY public.widgets (id, name) FROM stdin;
1	one
2	SET transaction_timeout = 0;
\.
SET transaction_timeout = 0;
SELECT 1;
`
	got, err := io.ReadAll(NewSQLFilter(strings.NewReader(input)))
	if err != nil {
		t.Fatal(err)
	}
	output := string(got)

	if strings.Contains(output, "SET transaction_timeout = 0;\nCOPY") {
		t.Fatalf("preamble setting was not filtered:\n%s", output)
	}
	if !strings.Contains(output, "SET statement_timeout = 0;") {
		t.Fatalf("unrelated preamble setting was removed:\n%s", output)
	}
	if !strings.Contains(output, "2\tSET transaction_timeout = 0;\n") {
		t.Fatalf("COPY data was modified:\n%s", output)
	}
	if !strings.Contains(output, "\\.\nSET transaction_timeout = 0;") {
		t.Fatalf("post-data setting should be preserved:\n%s", output)
	}
}

func TestSQLFilterStopsFilteringAtInsertData(t *testing.T) {
	input := `SET transaction_timeout = 0;
INSERT INTO public.widgets (id, name) VALUES (1, 'one');
SET transaction_timeout = 0;
`
	got, err := io.ReadAll(NewSQLFilter(strings.NewReader(input)))
	if err != nil {
		t.Fatal(err)
	}
	output := string(got)
	if strings.HasPrefix(output, "SET transaction_timeout") {
		t.Fatalf("preamble setting was not filtered:\n%s", output)
	}
	if strings.Count(output, "SET transaction_timeout = 0;") != 1 {
		t.Fatalf("post-data setting should be preserved exactly once:\n%s", output)
	}
}

func TestSQLFilterHandlesEOFWithoutNewline(t *testing.T) {
	got, err := io.ReadAll(NewSQLFilter(strings.NewReader("SELECT 1;")))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "SELECT 1;" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestIsTransactionTimeoutError(t *testing.T) {
	if !IsTransactionTimeoutError(`ERROR: unrecognized configuration parameter "transaction_timeout"`) {
		t.Fatal("expected known error to be detected")
	}
	if IsTransactionTimeoutError("some other error") {
		t.Fatal("unexpected detection")
	}
}
