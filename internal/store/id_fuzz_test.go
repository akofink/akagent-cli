package store

import "testing"

func FuzzTaskIDValidator(f *testing.F) {
	for _, seed := range []string{
		"018f47a5-1234-7abc-8def-0123456789ab",
		"00000000-0000-7000-8000-000000000000",
		"not-a-uuid", "", "../escape", "with space", "a/b", "\x00", "é",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, id string) {
		if id == "018f47a5-1234-7abc-8def-0123456789ab" && validateTaskID(id) != nil {
			t.Fatalf("valid UUIDv7 rejected")
		}
		_ = validateTaskID(id)
	})
}

func FuzzExecutionIDValidator(f *testing.F) {
	fuzzStableID(f, validateExecutionID)
}

func FuzzResourceIDValidator(f *testing.F) {
	fuzzStableID(f, validateResourceID)
}

func fuzzStableID(f *testing.F, validate func(string) error) {
	for _, seed := range []string{
		"impl-1", "resource_2", "a", "A.B-c_9", "", "../escape",
		"with space", "a/b", "a\\b", "..", "-leading", "trailing-",
		"\x00", "line\nbreak", "é", "a" + string(make([]byte, 1025)),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, id string) {
		_ = validate(id)
	})
}
