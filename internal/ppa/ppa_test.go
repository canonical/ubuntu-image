package ppa

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
)

var (
	imageDefPPA1 = &imagedefinition.PPA{
		Name:        "canonical-foundations/ubuntu-image",
		Auth:        "sil2100:vVg74j6SM8WVltwpxDRJ",
		Fingerprint: "CDE5112BD4104F975FC8A53FD4C0B668FD4C9139",
		KeepEnabled: helper.BoolPtr(false),
	}
	imageDefPPA2 = &imagedefinition.PPA{
		Name:        "canonical-foundations/ubuntu-image",
		Fingerprint: "",
		KeepEnabled: helper.BoolPtr(true),
	}

	imageDefPPA3 = &imagedefinition.PPA{
		Name:        "canonical-foundations/ubuntu-image-private-test",
		Auth:        "sil2100:vVg74j6SM8WVltwpxDRJ",
		Fingerprint: "CDE5112BD4104F975FC8A53FD4C0B668FD4C9139",
		KeepEnabled: helper.BoolPtr(false),
	}

	deb822PPA1Content = `Types: deb
URIS: https://sil2100:vVg74j6SM8WVltwpxDRJ@private-ppa.launchpadcontent.net/canonical-foundations/ubuntu-image/ubuntu
Suites: jammy
Components: main
Signed-By:
 -----BEGIN PGP PUBLIC KEY BLOCK-----
 .
 mI0EUL4ncAEEAOZssKpJDMZKbmsf9lHwlKA0vN6yQ0sOIPc500waH3xTC0sVlqQc
 3pUxCIdhU+qK1mH2D51FGHDb504k0Lpb+LE56TWa/X3xrZqUQX0UD1fykEruR4W2
 CdkXXZvmNBNatE9GurR6p407X5TED+dlUK/hIKNCb5unTEilBb4WwArxABEBAAG0
 LExhdW5jaHBhZCBQUEEgZm9yIENhbm9uaWNhbCBGb3VuZGF0aW9ucyBUZWFtiLgE
 EwECACIFAlC+J3ACGwMGCwkIBwMCBhUIAgkKCwQWAgMBAh4BAheAAAoJENTAtmj9
 TJE5u/MD/2j2auOv62YUFwT7POylj7ihhZOarOSCEiQGita8II77j5AoK5O75uD+
 oQc5pdxVN2NGYD5R0PmDCPFN1Rb869YjtsPgLefEB+6Tc1GOR9hgnwuSU5lrwqdQ
 Ht/skh2wZSHtJgejt9kqIKMho1wtYz7ZTqMtN9GJK0VONbHP0Xu6
 =Cfxk
 -----END PGP PUBLIC KEY BLOCK-----
`

	deb822PPA3Content = `Types: deb
URIS: https://sil2100:vVg74j6SM8WVltwpxDRJ@private-ppa.launchpadcontent.net/canonical-foundations/ubuntu-image-private-test/ubuntu
Suites: jammy
Components: main
Signed-By:
 -----BEGIN PGP PUBLIC KEY BLOCK-----
 .
 mI0EUL4ncAEEAOZssKpJDMZKbmsf9lHwlKA0vN6yQ0sOIPc500waH3xTC0sVlqQc
 3pUxCIdhU+qK1mH2D51FGHDb504k0Lpb+LE56TWa/X3xrZqUQX0UD1fykEruR4W2
 CdkXXZvmNBNatE9GurR6p407X5TED+dlUK/hIKNCb5unTEilBb4WwArxABEBAAG0
 LExhdW5jaHBhZCBQUEEgZm9yIENhbm9uaWNhbCBGb3VuZGF0aW9ucyBUZWFtiLgE
 EwECACIFAlC+J3ACGwMGCwkIBwMCBhUIAgkKCwQWAgMBAh4BAheAAAoJENTAtmj9
 TJE5u/MD/2j2auOv62YUFwT7POylj7ihhZOarOSCEiQGita8II77j5AoK5O75uD+
 oQc5pdxVN2NGYD5R0PmDCPFN1Rb869YjtsPgLefEB+6Tc1GOR9hgnwuSU5lrwqdQ
 Ht/skh2wZSHtJgejt9kqIKMho1wtYz7ZTqMtN9GJK0VONbHP0Xu6
 =Cfxk
 -----END PGP PUBLIC KEY BLOCK-----
`

	legacyPPA2Content = "deb https://ppa.launchpadcontent.net/canonical-foundations/ubuntu-image/ubuntu jammy main"
)

func mockMkdirAll(string, os.FileMode) error {
	return fmt.Errorf("os.MkdirAll error")
}

func mockMkdirTemp(string, string) (string, error) {
	return "", fmt.Errorf("os.MkdirTemp error")
}

func mockOpenFile(string, int, os.FileMode) (*os.File, error) {
	return nil, fmt.Errorf("os.OenFile error")
}

func mockRemove(string) error {
	return fmt.Errorf("os.Remove error")
}

func mockRemoveAll(string) error {
	return fmt.Errorf("os.RemoveAll error")
}

// wrapMkdirTemp returns a os.MkdirTemp wrapper to make sure tests will not create
// dirs outside of the test tmp dir
func wrapMkdirTemp(constDir string) func(string, string) (string, error) {
	return func(dir, pattern string) (string, error) {
		return os.MkdirTemp(constDir, pattern)
	}
}

var cmpOpts = []cmp.Option{
	cmp.AllowUnexported(
		LegacyPPA{},
		Deb822PPA{},
		BasePPA{},
	),
}

func TestNew(t *testing.T) {
	type args struct {
		imageDefPPA *imagedefinition.PPA
		deb822      bool
		series      string
	}
	tests := []struct {
		name string
		args args
		want PPAInterface
	}{
		{
			name: "instantiate a LegacyPPA",
			args: args{
				imageDefPPA: imageDefPPA1,
				deb822:      false,
				series:      "jammy",
			},
			want: &PPA{
				PPAPrivateInterface: &LegacyPPA{
					BasePPA: BasePPA{
						PPA:    imageDefPPA1,
						series: "jammy",
					},
				},
			},
		},
		{
			name: "instantiate a deb822",
			args: args{
				imageDefPPA: imageDefPPA1,
				deb822:      true,
				series:      "jammy",
			},
			want: &PPA{
				PPAPrivateInterface: &Deb822PPA{
					BasePPA: BasePPA{
						PPA:    imageDefPPA1,
						series: "jammy",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			got := New(tt.args.imageDefPPA, tt.args.deb822, tt.args.series)
			asserter.AssertEqual(tt.want, got, cmpOpts...)
		})
	}
}

func Test_ensureFingerprint(t *testing.T) {
	testCases := []struct {
		name                 string
		respondedFingerprint string
		currentFingerprint   string
		wantFingerprint      string
		expectedError        string
	}{
		{
			name:                 "valid key",
			respondedFingerprint: `{"signing_key_fingerprint":"CDE5112BD4104F975FC8A53FD4C0B668FD4C9139"}`,
			wantFingerprint:      "CDE5112BD4104F975FC8A53FD4C0B668FD4C9139",
		},
		{
			name:                 "non-empty fingerprint",
			currentFingerprint:   "CDE5112BD4104F975FC8A53FD4C0B668FD4C9139",
			respondedFingerprint: `{"signing_key_fingerprint":"test"}`,
			wantFingerprint:      "CDE5112BD4104F975FC8A53FD4C0B668FD4C9139",
		},
		{
			name:                 "invalid JSON string",
			respondedFingerprint: `test`,
			wantFingerprint:      "",
			expectedError:        "Error unmarshalling launchpad API response",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := io.WriteString(w, tc.respondedFingerprint)
				if err != nil {
					t.Error(err)
				}
			}))
			defer ts.Close()

			ppa := &BasePPA{
				series: "jammy",
				PPA: &imagedefinition.PPA{
					Name:        "lpuser/ppaname",
					Fingerprint: tc.currentFingerprint,
				},
			}

			err := ppa.ensureFingerprint(ts.URL)

			if len(tc.expectedError) == 0 {
				asserter.AssertErrNil(err, true)
				asserter.AssertEqual(tc.wantFingerprint, ppa.Fingerprint)
			} else {
				asserter.AssertErrContains(err, tc.expectedError)
			}
		})
	}
}

func TestLegacyPPA(t *testing.T) {
	asserter := helper.Asserter{T: t}
	p := &LegacyPPA{
		BasePPA: BasePPA{
			PPA:    imageDefPPA2,
			series: "jammy",
		},
	}
	asserter.AssertEqual("canonical-foundations-ubuntu-ubuntu-image-jammy.list", p.FileName())

	c, err := p.FileContent()
	asserter.AssertErrNil(err, true)

	wantContent := "deb https://ppa.launchpadcontent.net/canonical-foundations/ubuntu-image/ubuntu jammy main"
	asserter.AssertEqual(wantContent, c)
}

func TestLegacyAddRemove(t *testing.T) {
	asserter := helper.Asserter{T: t}
	p := &PPA{
		PPAPrivateInterface: &LegacyPPA{
			BasePPA: BasePPA{
				PPA:    imageDefPPA2,
				series: "jammy",
			},
		},
	}
	tmpDirPath, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(tmpDirPath) })

	gpgDir := filepath.Join(tmpDirPath, trustedGPGDPath)
	err = os.MkdirAll(gpgDir, 0755)
	asserter.AssertErrNil(err, true)

	err = p.Add(tmpDirPath, true)
	asserter.AssertErrNil(err, true)

	ppaFile := filepath.Join(tmpDirPath, sourcesListDPath, "canonical-foundations-ubuntu-ubuntu-image-jammy.list")
	ppaBytes, err := os.ReadFile(ppaFile)
	asserter.AssertErrNil(err, true)
	asserter.AssertEqual(legacyPPA2Content, string(ppaBytes))

	tmpGPGDir := filepath.Join(tmpDirPath, "tmp", "u-i-gpg")
	_, err = os.Stat(tmpGPGDir)
	if !os.IsNotExist(err) {
		t.Errorf("Dir %s should not exist, but does", tmpGPGDir)
	}

	err = p.Remove(tmpDirPath)
	asserter.AssertErrNil(err, true)

	_, err = os.Stat(ppaFile)
	if os.IsNotExist(err) {
		t.Errorf("File %s should exist, but does not", ppaFile)
	}
}

func TestDeb822PPA(t *testing.T) {
	asserter := helper.Asserter{T: t}

	tmpDirPath, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(tmpDirPath) })

	p := &Deb822PPA{
		BasePPA{
			PPA:    imageDefPPA3,
			series: "jammy",
		},
	}
	asserter.AssertEqual("canonical-foundations-ubuntu-ubuntu-image-private-test-jammy.sources", p.FileName())

	_, err = p.FileContent()
	asserter.AssertErrContains(err, "received an empty signing key for PPA")

	err = p.ImportKey(tmpDirPath, true)
	asserter.AssertErrNil(err, true)

	c, err := p.FileContent()
	asserter.AssertErrNil(err, true)
	asserter.AssertEqual(deb822PPA3Content, c)

	gotName := p.FullName()
	asserter.AssertEqual(imageDefPPA3.Name, gotName)
}

func TestDeb822AddRemove(t *testing.T) {
	asserter := helper.Asserter{T: t}
	p := &PPA{
		PPAPrivateInterface: &Deb822PPA{
			BasePPA{
				PPA:    imageDefPPA1,
				series: "jammy",
			},
		},
	}
	tmpDirPath, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(tmpDirPath) })
	err = p.Add(tmpDirPath, true)
	asserter.AssertErrNil(err, true)

	ppaFile := filepath.Join(tmpDirPath, sourcesListDPath, "canonical-foundations-ubuntu-ubuntu-image-jammy.sources")
	ppaBytes, err := os.ReadFile(ppaFile)
	asserter.AssertErrNil(err, true)
	asserter.AssertEqual(deb822PPA1Content, string(ppaBytes))

	err = p.Remove(tmpDirPath)
	asserter.AssertErrNil(err, true)

	_, err = os.Stat(ppaFile)
	if !os.IsNotExist(err) {
		t.Errorf("File %s should not exist, but does", ppaFile)
	}
}

func TestAdd_fail(t *testing.T) {
	asserter := helper.Asserter{T: t}
	p := &PPA{
		PPAPrivateInterface: &LegacyPPA{
			BasePPA{
				PPA:    imageDefPPA1,
				series: "jammy",
			},
		},
	}
	tmpDirPath, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(tmpDirPath) })

	gpgDir := filepath.Join(tmpDirPath, trustedGPGDPath)
	err = os.MkdirAll(gpgDir, 0755)
	asserter.AssertErrNil(err, true)

	osMkdirTemp = wrapMkdirTemp(tmpDirPath)
	t.Cleanup(func() {
		osMkdirTemp = os.MkdirTemp
	})

	osMkdirAll = mockMkdirAll
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	err = p.Add(tmpDirPath, true)
	asserter.AssertErrContains(err, "Failed to create apt sources.list.d")
	osMkdirAll = os.MkdirAll

	osMkdirTemp = mockMkdirTemp
	t.Cleanup(func() {
		osMkdirTemp = os.MkdirTemp
	})
	err = p.Add(tmpDirPath, true)
	asserter.AssertErrContains(err, "Error creating temp dir for gpg imports")
	osMkdirTemp = os.MkdirTemp

	osOpenFile = mockOpenFile
	t.Cleanup(func() {
		osOpenFile = os.OpenFile
	})
	err = p.Add(tmpDirPath, true)
	asserter.AssertErrContains(err, "Error creating")
	osOpenFile = os.OpenFile

	os.RemoveAll(gpgDir)
	err = os.MkdirAll(gpgDir, 0755)
	asserter.AssertErrNil(err, true)

	osRemoveAll = mockRemoveAll
	defer func() {
		osRemoveAll = os.RemoveAll
	}()
	err = p.Add(tmpDirPath, true)
	asserter.AssertErrContains(err, "Error removing temporary gpg directory")
	osRemoveAll = os.RemoveAll

}

func TestRemove_fail(t *testing.T) {
	asserter := helper.Asserter{T: t}
	tmpDirPath, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(tmpDirPath) })

	p := &PPA{
		PPAPrivateInterface: &Deb822PPA{
			BasePPA{
				PPA: &imagedefinition.PPA{
					Name:        "canonical-foundations/ubuntu-image",
					Auth:        "sil2100:vVg74j6SM8WVltwpxDRJ",
					Fingerprint: "CDE5112BD4104F975FC8A53FD4C0B668FD4C9139",
					KeepEnabled: nil,
				},
				series: "jammy",
			},
		},
	}

	err = p.Remove(tmpDirPath)
	asserter.AssertErrContains(err, imagedefinition.ErrKeepEnabledNil.Error())

	p = &PPA{
		PPAPrivateInterface: &Deb822PPA{
			BasePPA{
				PPA:    imageDefPPA1,
				series: "jammy",
			},
		},
	}

	osRemove = mockRemove
	t.Cleanup(func() {
		osRemove = os.Remove
	})

	err = p.Remove(tmpDirPath)
	asserter.AssertErrContains(err, "Error removing")
	osRemove = os.Remove
}

func Test_formatKey(t *testing.T) {
	asserter := helper.Asserter{T: t}

	testCases := []struct {
		name          string
		rawKey        string
		mockFuncs     func() func()
		wantKey       string
		expectedError string
	}{
		{
			name: "valid key",
			rawKey: `-----BEGIN PGP PUBLIC KEY BLOCK-----

mI0EUL4ncAEEAOZssKpJDMZKbmsf9lHwlKA0vN6yQ0sOIPc500waH3xTC0sVlqQc
3pUxCIdhU+qK1mH2D51FGHDb504k0Lpb+LE56TWa/X3xrZqUQX0UD1fykEruR4W2
CdkXXZvmNBNatE9GurR6p407X5TED+dlUK/hIKNCb5unTEilBb4WwArxABEBAAG0
LExhdW5jaHBhZCBQUEEgZm9yIENhbm9uaWNhbCBGb3VuZGF0aW9ucyBUZWFtiLgE
EwECACIFAlC+J3ACGwMGCwkIBwMCBhUIAgkKCwQWAgMBAh4BAheAAAoJENTAtmj9
TJE5u/MD/2j2auOv62YUFwT7POylj7ihhZOarOSCEiQGita8II77j5AoK5O75uD+
oQc5pdxVN2NGYD5R0PmDCPFN1Rb869YjtsPgLefEB+6Tc1GOR9hgnwuSU5lrwqdQ
Ht/skh2wZSHtJgejt9kqIKMho1wtYz7ZTqMtN9GJK0VONbHP0Xu6
=Cfxk
-----END PGP PUBLIC KEY BLOCK-----`,
			wantKey: ` -----BEGIN PGP PUBLIC KEY BLOCK-----
 .
 mI0EUL4ncAEEAOZssKpJDMZKbmsf9lHwlKA0vN6yQ0sOIPc500waH3xTC0sVlqQc
 3pUxCIdhU+qK1mH2D51FGHDb504k0Lpb+LE56TWa/X3xrZqUQX0UD1fykEruR4W2
 CdkXXZvmNBNatE9GurR6p407X5TED+dlUK/hIKNCb5unTEilBb4WwArxABEBAAG0
 LExhdW5jaHBhZCBQUEEgZm9yIENhbm9uaWNhbCBGb3VuZGF0aW9ucyBUZWFtiLgE
 EwECACIFAlC+J3ACGwMGCwkIBwMCBhUIAgkKCwQWAgMBAh4BAheAAAoJENTAtmj9
 TJE5u/MD/2j2auOv62YUFwT7POylj7ihhZOarOSCEiQGita8II77j5AoK5O75uD+
 oQc5pdxVN2NGYD5R0PmDCPFN1Rb869YjtsPgLefEB+6Tc1GOR9hgnwuSU5lrwqdQ
 Ht/skh2wZSHtJgejt9kqIKMho1wtYz7ZTqMtN9GJK0VONbHP0Xu6
 =Cfxk
 -----END PGP PUBLIC KEY BLOCK-----`,
		},
		{
			name:          "empty key",
			rawKey:        "",
			wantKey:       "",
			expectedError: "received an empty signing key for PPA lpuser/ppaname",
		},
	}
	for _, tc := range testCases {
		t.Run("test_generate_apt_cmd_"+tc.name, func(t *testing.T) {
			ppa := &Deb822PPA{
				BasePPA: BasePPA{
					series: "jammy",
					PPA: &imagedefinition.PPA{
						Name: "lpuser/ppaname",
					},
				},
			}

			key, err := ppa.formatKey(tc.rawKey)

			if len(tc.expectedError) == 0 {
				asserter.AssertErrNil(err, true)
				asserter.AssertEqual(tc.wantKey, key)
			} else {
				asserter.AssertErrContains(err, tc.expectedError)
			}
		})
	}
}
