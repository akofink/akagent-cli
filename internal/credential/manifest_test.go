package credential

import (
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	data := []byte(`version: 1
credentials[2]{id,type,source,required_for}:
  git-ssh,ssh_key,file:/secrets/git,git
  gh-token,api_token,env:GITHUB_TOKEN,
`)
	manifest, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if manifest.Version != 1 {
		t.Errorf("Version = %d, want 1", manifest.Version)
	}
	if len(manifest.Entries) != 2 {
		t.Fatalf("len(Entries) = %d, want 2", len(manifest.Entries))
	}

	git := manifest.Entries[0]
	if git.ID != "git-ssh" || git.Type != "ssh_key" || git.Source != "file:/secrets/git" || git.RequiredFor != "git" {
		t.Errorf("entry[0] = %+v", git)
	}
	if !git.Required() {
		t.Errorf("entry[0] should be required")
	}

	gh := manifest.Entries[1]
	if gh.ID != "gh-token" || gh.Kind() != KindEnv || gh.Ref() != "GITHUB_TOKEN" || gh.RequiredFor != "" {
		t.Errorf("entry[1] = %+v (kind=%q ref=%q)", gh, gh.Kind(), gh.Ref())
	}
	if gh.Required() {
		t.Errorf("entry[1] should be optional")
	}
}

func TestParseDefaultsVersionWithoutHeader(t *testing.T) {
	manifest, err := Parse([]byte("credentials{id,type,source,required_for}:\n  a,token,env:A,\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if manifest.Version != 1 {
		t.Errorf("Version = %d, want default 1", manifest.Version)
	}
	if len(manifest.Entries) != 1 || manifest.Entries[0].ID != "a" {
		t.Errorf("entries = %+v", manifest.Entries)
	}
}

func TestParseEmptyIsDefinitiveEmptyState(t *testing.T) {
	manifest, err := Parse([]byte(""))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(manifest.Entries) != 0 {
		t.Errorf("len(Entries) = %d, want 0", len(manifest.Entries))
	}
}

func TestParseMalformedHeader(t *testing.T) {
	for _, data := range []string{
		"credentials[2]{id,type}:",
		"credentials{id,source}:",
		"credentials[2]id,type,source,required_for:",
		"credentials[x]{id,type,source,required_for}:",
	} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("Parse(%q) expected error", data)
		}
	}
}

func TestParseWrongFieldCount(t *testing.T) {
	data := []byte("credentials{id,type,source,required_for}:\n  id,type,source\n")
	if _, err := Parse(data); err == nil {
		t.Fatal("Parse() expected error for wrong field count")
	}
}

func TestParseDeclaredLengthMismatch(t *testing.T) {
	data := []byte("credentials[1]{id,type,source,required_for}:\n  a,t,env:A,x\n  b,t,env:B,\n")
	if _, err := Parse(data); err == nil {
		t.Fatal("Parse() expected error for declared length mismatch")
	}
}

func TestParseUnsupportedSourceKind(t *testing.T) {
	data := []byte("credentials{id,type,source,required_for}:\n  sign,gpg_key,gpg:SUBKEY,commit\n")
	if _, err := Parse(data); err == nil {
		t.Fatal("Parse() expected error for unsupported source kind")
	}
}

func TestParseEmptyIDOrSource(t *testing.T) {
	for _, data := range []string{
		"credentials{id,type,source,required_for}:\n  ,t,env:A,\n",
		"credentials{id,type,source,required_for}:\n  a,t,,\n",
	} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("Parse(%q) expected error", data)
		}
	}
}

func TestParseUnexpectedLine(t *testing.T) {
	data := []byte("not-credentials\n")
	if _, err := Parse(data); err == nil {
		t.Fatal("Parse() expected error for unexpected line")
	}
}

func TestParseInvalidVersion(t *testing.T) {
	data := []byte("version: nope\n")
	if _, err := Parse(data); err == nil {
		t.Fatal("Parse() expected error for invalid version")
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	manifest, err := Load("/definitely/not/a/real/manifest.toon")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(manifest.Entries) != 0 {
		t.Errorf("len(Entries) = %d, want 0", len(manifest.Entries))
	}
}

func TestParseQuotedFieldsCommasWhitespaceQuotesAndEscapes(t *testing.T) {
	data := []byte("credentials[1]{id,type,source,required_for}:\n  \"id,one\",\" type with spaces \",\"env:VAR\\\"NAME\",\"capability,one\"\n")
	manifest, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(manifest.Entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1", len(manifest.Entries))
	}
	entry := manifest.Entries[0]
	if entry.ID != "id,one" || entry.Type != " type with spaces " || entry.Source != "env:VAR\"NAME" || entry.RequiredFor != "capability,one" {
		t.Fatalf("entry = %+v, want quoted fields preserved and decoded", entry)
	}
}

func TestParseQuotedEscapes(t *testing.T) {
	data := []byte("credentials[1]{id,type,source,required_for}:\n  \"id\\\\one\",token,env:VAR,\"line1\\nline2\"\n")
	manifest, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if manifest.Entries[0].ID != `id\one` || manifest.Entries[0].RequiredFor != "line1\nline2" {
		t.Fatalf("entry = %+v, want escaped values decoded", manifest.Entries[0])
	}
}

func TestParseMalformedRowErrorDoesNotEchoManifestContent(t *testing.T) {
	secret := "super-secret-manifest-value"
	data := []byte("credentials[1]{id,type,source,required_for}:\n  \"" + secret + "\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("Parse() expected malformed-row error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Parse() error leaked manifest content: %q", err)
	}
}

func TestParseRejectsUnsupportedVersion(t *testing.T) {
	for _, version := range []string{"0", "2", "999"} {
		data := []byte("version: " + version + "\n")
		if _, err := Parse(data); err == nil {
			t.Errorf("Parse(version %s) expected error", version)
		}
	}
}

func TestParseRejectsDuplicateIDs(t *testing.T) {
	data := []byte("credentials[2]{id,type,source,required_for}:\n  same,token,env:A,\n  same,token,env:B,\n")
	if _, err := Parse(data); err == nil {
		t.Fatal("Parse() expected duplicate-ID error")
	}
}

func TestParseRejectsQuotedFieldSuffix(t *testing.T) {
	data := []byte("credentials[1]{id,type,source,required_for}:\n  id,token,\"env:TOKEN\"suffix,\n")
	if _, err := Parse(data); err == nil {
		t.Fatal("Parse() accepted quoted field with trailing unquoted suffix")
	}
}
