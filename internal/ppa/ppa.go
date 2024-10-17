// Package ppa manages Private Package Archives sources list.
// It enables adding and removing a PPA on a system.
package ppa

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
)

var (
	httpGet       = http.Get
	ioReadAll     = io.ReadAll
	jsonUnmarshal = json.Unmarshal
	osRemove      = os.Remove
	osRemoveAll   = os.RemoveAll
	osMkdirAll    = os.MkdirAll
	osMkdirTemp   = os.MkdirTemp
	osOpenFile    = os.OpenFile
	execCommand   = exec.Command

	sourcesListDPath = filepath.Join("etc", "apt", "sources.list.d")
	trustedGPGDPath  = filepath.Join("etc", "apt", "trusted.gpg.d")
	lpBaseURL        = "https://api.launchpad.net"
)

// PPAInterface is the only interface that should be used outside of this package.
// It defines the behavior of a PPA.
type PPAInterface interface {
	Add(basePath string, debug bool) error
	Remove(basePath string) error
}

// PPAPrivateInterface defines internal behavior expected from a PPAInterface implementer.
// Even though methods are exporter, they are not meant to be used outside of this package.
type PPAPrivateInterface interface {
	FullName() string
	FileName() string
	FileContent() (string, error)
	ImportKey(basePath string, debug bool) error
	Remove(basePath string) error
}

// New instantiates the proper PPA implementation based on the deb822 flag
func New(imageDefPPA *imagedefinition.PPA, deb822 bool, series string) PPAInterface {
	basePPA := BasePPA{
		PPA:    imageDefPPA,
		series: series,
	}

	if deb822 {
		return &PPA{
			PPAPrivateInterface: &Deb822PPA{
				BasePPA: basePPA,
			},
		}
	}

	return &PPA{
		PPAPrivateInterface: &LegacyPPA{
			BasePPA: basePPA,
		},
	}
}

// BasePPA holds fields and methods common to every PPAPrivateInterface implementation
type BasePPA struct {
	*imagedefinition.PPA
	series     string
	signingKey string
}

func (p *BasePPA) FullName() string {
	return p.Name
}

func (p *BasePPA) name() string {
	return strings.Split(p.Name, "/")[1]
}

func (p *BasePPA) user() string {
	return strings.Split(p.Name, "/")[0]
}

func (p *BasePPA) url() string {
	var baseURL string
	if p.Auth == "" {
		baseURL = "https://ppa.launchpadcontent.net"
	} else {
		baseURL = fmt.Sprintf("https://%s@private-ppa.launchpadcontent.net", p.Auth)
	}
	return fmt.Sprintf("%s/%s/%s/ubuntu", baseURL, p.user(), p.name())
}

// removePPAFile removes the PPA file from the sources.list.d directory
func (p *BasePPA) removePPAFile(basePath string, fileName string) error {
	sourcesListD := filepath.Join(basePath, sourcesListDPath)
	if p.KeepEnabled == nil {
		return imagedefinition.ErrKeepEnabledNil
	}

	if *p.KeepEnabled {
		return nil
	}

	ppaFile := filepath.Join(sourcesListD, fileName)
	err := osRemove(ppaFile)
	if err != nil {
		return fmt.Errorf("Error removing %s: %s", ppaFile, err.Error())
	}
	return nil
}

// importKey fetches and imports the public key of a PPA.
// This function relies on gpg to fetch the key from the keyserver. We cannot reliably get this key
// from Launchpad because it is not publicly accessible for private PPAs.
// If the ascii arg is set to true, the key is also stored dearmored in the signingKey field of p.
func (p *BasePPA) importKey(basePath string, ppaFileName string, ascii bool, debug bool) (err error) {
	trustedGPGD := filepath.Join(basePath, trustedGPGDPath)
	keyFileName := strings.Replace(ppaFileName, ".list", ".gpg", 1)
	keyFilePath := filepath.Join(trustedGPGD, keyFileName)

	err = p.ensureFingerprint(lpBaseURL)
	if err != nil {
		return err
	}

	tmpGPGDir, err := p.createTmpGPGDir()
	if err != nil {
		return err
	}

	defer func() {
		tmpErr := osRemoveAll(tmpGPGDir)
		if tmpErr != nil {
			if err != nil {
				err = fmt.Errorf("%s after previous error: %w", tmpErr.Error(), err)
			} else {
				err = fmt.Errorf("Error removing temporary gpg directory \"%s\": %s", tmpGPGDir, tmpErr.Error())
			}
		}
	}()

	tmpASCIIKeyFilName := keyFileName + ".asc"
	tmpASCIIKeyFilePath := filepath.Join(tmpGPGDir, tmpASCIIKeyFilName)

	commonGPGArgs := []string{
		"--no-default-keyring",
		"--no-options",
		"--batch",
		"--homedir",
		tmpGPGDir,
		"--keyserver",
		"hkp://keyserver.ubuntu.com:80",
	}
	recvKeyArgs := append(commonGPGArgs, "--recv-keys", p.Fingerprint)

	exportKeyArgs := make([]string, 0)
	exportKeyArgs = append(exportKeyArgs, commonGPGArgs...)

	if ascii {
		exportKeyArgs = append(exportKeyArgs, "-a", "--output", tmpASCIIKeyFilePath)
	} else {
		exportKeyArgs = append(exportKeyArgs, "--output", keyFilePath)
	}

	exportKeyArgs = append(exportKeyArgs, "--export", p.Fingerprint)

	gpgCmds := []*exec.Cmd{
		execCommand(
			"gpg",
			recvKeyArgs...,
		),
		execCommand(
			"gpg",
			exportKeyArgs...,
		),
	}

	for _, gpgCmd := range gpgCmds {
		gpgOutput := helper.SetCommandOutput(gpgCmd, debug)
		err := gpgCmd.Run()
		if err != nil {
			err = fmt.Errorf("Error running gpg command \"%s\". Error is \"%s\". Full output below:\n%s",
				gpgCmd.String(), err.Error(), gpgOutput.String())
			return err
		}
	}

	keyBytes := []byte{}
	if ascii {
		keyBytes, err = os.ReadFile(tmpASCIIKeyFilePath)
		if err != nil {
			return err
		}
	}

	p.signingKey = string(keyBytes)

	return nil
}

// ensureFingerprint ensures a non empty fingerprint is set on the PPA object
// Fingerprint for private PPA cannot be fetched, so they have to be provided in
// the configuration.
func (p *BasePPA) ensureFingerprint(baseURL string) error {
	if p.PPA.Fingerprint != "" {
		return nil
	}
	// The YAML schema has already been validated that if no fingerprint is
	// provided, then this is a public PPA. We will get the fingerprint
	// from the Launchpad API
	type lpResponse struct {
		SigningKeyFingerprint string `json:"signing_key_fingerprint"`
		// plus many other fields that aren't needed at the moment
	}
	lpRespContent := &lpResponse{}

	lpURL := fmt.Sprintf("%s/devel/~%s/+archive/ubuntu/%s", baseURL,
		p.user(), p.name())

	resp, err := httpGet(lpURL)
	if err != nil {
		return fmt.Errorf("Error getting signing key for ppa \"%s\": %s",
			p.name(), err.Error())
	}
	defer resp.Body.Close()

	body, err := ioReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error reading signing key for ppa \"%s\": %s",
			p.name(), err.Error())
	}

	err = jsonUnmarshal(body, lpRespContent)
	if err != nil {
		return fmt.Errorf("Error unmarshalling launchpad API response: %s", err.Error())
	}

	p.Fingerprint = lpRespContent.SigningKeyFingerprint

	return nil
}

func (p *BasePPA) createTmpGPGDir() (string, error) {
	tmpGPGDir, err := osMkdirTemp("", "u-i-gpg")
	if err != nil {
		return "", fmt.Errorf("Error creating temp dir for gpg imports: %s", err.Error())
	}

	return tmpGPGDir, nil
}

// PPA is a basic implementation of the PPAPrivateInterface enabling
// the implementation of common behaviors between LegacyPPA and Deb822PPA
type PPA struct {
	PPAPrivateInterface
}

// Add adds the PPA to the sources.list.d directory and imports the signing key.
func (p *PPA) Add(basePath string, debug bool) error {
	sourcesListD := filepath.Join(basePath, sourcesListDPath)
	err := osMkdirAll(sourcesListD, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Failed to create apt sources.list.d: %s", err.Error())
	}

	err = p.ImportKey(basePath, debug)
	if err != nil {
		return fmt.Errorf("Error retrieving signing key for ppa \"%s\": %s",
			p.FullName(), err.Error())
	}

	var ppaIO *os.File
	ppaFile := filepath.Join(sourcesListD, p.FileName())
	ppaIO, err = osOpenFile(ppaFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("Error creating %s: %s", ppaFile, err.Error())
	}
	defer ppaIO.Close()

	content, err := p.FileContent()
	if err != nil {
		return err
	}

	_, err = ppaIO.Write([]byte(content))
	if err != nil {
		return fmt.Errorf("unable to write ppa file %s: %w", ppaFile, err)
	}

	return nil
}

// LegacyPPA implements behaviors to manage PPA the legacy way, specifically:
// - write in a sources.list file
// - manage signing key with gpg in /etc/apt/trusted.gpg.d
type LegacyPPA struct {
	BasePPA
}

func (p *LegacyPPA) FileName() string {
	return fmt.Sprintf("%s-ubuntu-%s-%s.list", p.user(), p.name(), p.series)
}

func (p *LegacyPPA) FileContent() (string, error) {
	return fmt.Sprintf("deb %s %s main", p.url(), p.series), nil
}

func (p *LegacyPPA) ImportKey(basePath string, debug bool) error {
	return p.BasePPA.importKey(basePath, p.FileName(), false, debug)
}

func (p *LegacyPPA) Remove(basePath string) error {
	err := p.removePPAFile(basePath, p.FileName())
	if err != nil {
		return err
	}

	trustedGPGD := filepath.Join(basePath, trustedGPGDPath)
	keyFileName := strings.Replace(p.FileName(), ".list", ".gpg", 1)
	keyFilePath := filepath.Join(trustedGPGD, keyFileName)

	err = osRemove(keyFilePath)
	if err != nil {
		return fmt.Errorf("Error removing %s: %s", keyFilePath, err.Error())
	}

	return nil
}

// Deb822PPA implements behaviors to manage PPA in the deb822 format, specifically:
// - write in a <ppa>.sources file, in the deb822 format
// - embed the signing key in the file itself
type Deb822PPA struct {
	BasePPA
}

func (p *Deb822PPA) FileName() string {
	return fmt.Sprintf("%s-ubuntu-%s-%s.sources", p.user(), p.name(), p.series)
}

func (p *Deb822PPA) FileContent() (string, error) {
	key, err := p.formatKey(p.BasePPA.signingKey)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Types: deb\n"+
		"URIS: %s\nSuites: %s\nComponents: main\nSigned-By:\n%s\n",
		p.url(), p.series, key), nil
}

func (p *Deb822PPA) ImportKey(basePath string, debug bool) error {
	return p.BasePPA.importKey(basePath, p.FileName(), true, debug)
}

func (p *Deb822PPA) Remove(basePath string) error {
	return p.removePPAFile(basePath, p.FileName())
}

// formatKey formats the signing key for a PPA to be set in a deb822
// formatted Signed-By field.
func (p *Deb822PPA) formatKey(rawKey string) (string, error) {
	rawKey = strings.TrimSpace(rawKey)
	if len(rawKey) == 0 {
		return "", fmt.Errorf("received an empty signing key for PPA %s", p.Name)
	}

	lines := make([]string, 0)
	for _, l := range strings.Split(rawKey, "\n") {
		if l == "" {
			lines = append(lines, " .")
		} else {
			lines = append(lines, " "+l)
		}
	}

	return strings.Join(lines, "\n"), nil
}
