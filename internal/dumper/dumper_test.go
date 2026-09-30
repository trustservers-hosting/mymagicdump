package dumper

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/trustservers-hosting/mymagicdump/internal/config"
)

func fakeClient(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestPrepareAllDatabasesSeparate(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("MYSQL_ARGS_FILE", argsFile)
	fakeClient(t, "mysql", `printf '%s\n' "$@" > "$MYSQL_ARGS_FILE"
printf '%s\n' 'Warning: test warning' >&2
printf '%s\n' information_schema performance_schema mysql shop analytics
`)
	defaultsFile := filepath.Join(t.TempDir(), "my.cnf")
	if err := os.WriteFile(defaultsFile, nil, 0600); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(&config.Options{
		AllDatabases: true, SeparateDumps: true,
		User: "backup", Password: "test-password", Host: "db.example",
		Port: "3307", Socket: "/tmp/mysql.sock", DefaultsFile: defaultsFile,
		DefaultsGroupSuffix: "_backup",
	})
	if err := r.Prepare(); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"-u", "backup", "-ptest-password", "-h", "db.example", "-P", "3307", "--socket", "/tmp/mysql.sock", "--defaults-file=" + defaultsFile, "--defaults-group-suffix=_backup", "-sNe", "SHOW DATABASES;"}
	if got := strings.Split(strings.TrimSuffix(string(args), "\n"), "\n"); !reflect.DeepEqual(got, wantArgs) {
		t.Fatalf("discovery args = %q, want %q", got, wantArgs)
	}
	want := [][]string{{"--databases", "mysql"}, {"--databases", "shop"}, {"--databases", "analytics"}}
	if !reflect.DeepEqual(r.DumpFlagsList, want) {
		t.Fatalf("dump commands = %q, want %q", r.DumpFlagsList, want)
	}
}

func TestPrepareAllDatabasesSeparateDiscoveryFailure(t *testing.T) {
	for _, tc := range []struct {
		name, script, wantError string
	}{
		{"connection failure", "printf 'Access denied for user backup' >&2\nexit 1\n", "Access denied for user backup"},
		{"empty result", "exit 0\n", "no databases available"},
		{"only virtual schemas", "printf '%s\\n' information_schema performance_schema\n", "no databases available"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient(t, "mysql", tc.script)
			r := NewRunner(&config.Options{AllDatabases: true, SeparateDumps: true})
			err := r.Prepare()
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("Prepare() error = %v, want %q", err, tc.wantError)
			}
			if len(r.DumpFlagsList) != 0 {
				t.Fatalf("unexpected dump commands after failed discovery: %q", r.DumpFlagsList)
			}
		})
	}
}

func TestPrepareExistingDatabaseModes(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts config.Options
		want [][]string
	}{
		{"combined all", config.Options{AllDatabases: true}, [][]string{{"--all-databases"}}},
		{"explicit separate", config.Options{SeparateDumps: true, Databases: config.CommaSeparatedList{"shop", "analytics"}}, [][]string{{"shop"}, {"analytics"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Neither mode should require discovery during preparation.
			t.Setenv("PATH", t.TempDir())
			r := NewRunner(&tc.opts)
			if err := r.Prepare(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(r.DumpFlagsList, tc.want) {
				t.Fatalf("dump commands = %q, want %q", r.DumpFlagsList, tc.want)
			}
		})
	}
}

func TestAllDatabasesSeparateSchemaAndData(t *testing.T) {
	opts := config.Options{
		AllDatabases: true, SeparateDumps: true,
		Databases:         config.CommaSeparatedList{"shop", "analytics"},
		ExcludeTablesData: config.CommaSeparatedList{"shop.audit"},
	}
	got := buildDumpFlags(opts, []string{"shop.temp"}, []string{"shop.audit"})
	want := [][]string{
		{"--ignore-table=shop.temp", "--databases", "--no-data", "--skip-triggers", "shop"},
		{"--ignore-table=shop.temp", "--databases", "--no-create-info", "--ignore-table=shop.audit", "shop"},
		{"--ignore-table=shop.temp", "--databases", "analytics"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dump commands = %q, want %q", got, want)
	}
}

func TestAllDatabasesSeparateProducesCompressedDumps(t *testing.T) {
	fakeClient(t, "mysql", `for arg do query="$arg"; done
if [ "$query" = 'SHOW DATABASES;' ]; then
  printf '%s\n' information_schema performance_schema shop analytics
else
  printf '42\n'
fi
`)
	fakeClient(t, "mysqldump", `for arg do database="$arg"; done
printf 'CREATE DATABASE %s;\n' "$database"
`)
	output := t.TempDir()
	r := NewRunner(&config.Options{AllDatabases: true, SeparateDumps: true, OutputPath: output, Compression: "tgz"})
	if err := r.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(output, "multiple_databases.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	got := map[string]string{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		got[header.Name] = string(content)
	}
	want := map[string]string{"shop.sql": "CREATE DATABASE shop;\n", "analytics.sql": "CREATE DATABASE analytics;\n"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("archive contents = %q, want %q", got, want)
	}
}
