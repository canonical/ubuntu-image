package helper

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/invopop/jsonschema"
	"github.com/klauspost/compress/zstd"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/ulikunitz/xz"
	"github.com/xeipuuv/gojsonschema"
)

// CaptureStd returns an io.Reader to read what was printed, and teardown
func CaptureStd(toCap **os.File) (io.Reader, func(), error) {
	stdCap, stdCapW, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	oldStdCap := *toCap
	*toCap = stdCapW
	closed := false
	return stdCap, func() {
		// only teardown once
		if closed {
			return
		}
		*toCap = oldStdCap
		stdCapW.Close()
		closed = true
	}, nil
}

// InitCommonOpts initializes default common options for state machines.
// This is used for test scenarios to avoid nil pointer dereferences
func InitCommonOpts() (*commands.CommonOpts, *commands.StateMachineOpts) {
	commonOpts := new(commands.CommonOpts)
	// This is a workaround to set the default value for test cases. Normally
	// go-flags makes sure that the option has a sane value at all times, but
	// for tests we'd have to set it manually all the time.
	commonOpts.SectorSize = "512"
	return commonOpts, new(commands.StateMachineOpts)
}

// RunScript runs scripts from disk. Currently only used for hooks
func RunScript(hookScript string) error {
	hookScriptCmd := exec.Command(hookScript)
	hookScriptCmd.Env = os.Environ()
	hookScriptCmd.Stdout = os.Stdout
	hookScriptCmd.Stderr = os.Stderr
	if err := hookScriptCmd.Run(); err != nil {
		return fmt.Errorf("Error running hook script %s: %s", hookScript, err.Error())
	}
	return nil
}

// SaveCWD gets the current working directory and returns a function to go back to it
func SaveCWD() func() {
	wd, _ := os.Getwd()
	return func() {
		os.Chdir(wd)
	}
}

// Du recurses through a directory similar to du and adds all the sizes of files together
func Du(path string) (quantity.Size, error) {
	duCommand := *exec.Command("du", "-s", "-B1")
	duCommand.Args = append(duCommand.Args, path)

	duBytes, err := duCommand.Output()
	if err != nil {
		return quantity.Size(0), err
	}
	sizeString := strings.Split(string(duBytes), "\t")[0]
	size, err := quantity.ParseSize(sizeString)
	return size, err
}

// CopyBlob runs `dd` to copy a blob to an image file
func CopyBlob(ddArgs []string) error {
	ddCommand := *exec.Command("dd")
	ddCommand.Args = append(ddCommand.Args, ddArgs...)

	if err := ddCommand.Run(); err != nil {
		return fmt.Errorf("Command \"%s\" returned with %s", ddCommand.String(), err.Error())
	}
	return nil
}

// SetDefaults iterates through the keys in a struct and sets
// default values if one is specified with a struct tag of "default".
// Currently only default values of strings, slice of strings, and
// bools are supported
func SetDefaults(needsDefaults interface{}) error {
	value := reflect.ValueOf(needsDefaults)
	if value.Kind() != reflect.Ptr {
		return fmt.Errorf("The argument to SetDefaults must be a pointer")
	}
	elem := value.Elem()
	for i := 0; i < elem.NumField(); i++ {
		field := elem.Field(i)
		// if we're dealing with a slice of pointers to structs,
		// iterate through it and set the defaults for each struct pointer
		if field.Type().Kind() == reflect.Slice &&
			field.Cap() > 0 &&
			field.Index(0).Kind() == reflect.Pointer {
			for i := 0; i < field.Cap(); i++ {
				SetDefaults(field.Index(i).Interface())
			}
		} else if field.Type().Kind() == reflect.Ptr {
			// otherwise if it's just a pointer, look for default types
			if field.Elem().Kind() == reflect.Struct {
				SetDefaults(field.Interface())
			}
		} else {
			tags := elem.Type().Field(i).Tag
			defaultValue, hasDefault := tags.Lookup("default")
			if hasDefault {
				indirectedField := reflect.Indirect(field)
				if indirectedField.CanSet() && field.IsZero() {
					varType := field.Type().Kind()
					switch varType {
					case reflect.String:
						field.SetString(defaultValue)
						break
					case reflect.Slice:
						defaultValues := strings.Split(defaultValue, ",")
						field.Set(reflect.ValueOf(defaultValues))
						break
					case reflect.Bool:
						if defaultValue == "true" {
							field.SetBool(true)
						} else {
							field.SetBool(false)
						}
						break
					default:
						return fmt.Errorf("Setting default value of type %s not supported",
							varType)
					}
				}
			}
		}
	}
	return nil
}

// CheckEmptyFields iterates through the image definition struct and
// checks for fields that are present but return IsZero == true.
// TODO: I've created a PR upstream in xeipuuv/gojsonschema
// https://github.com/xeipuuv/gojsonschema/pull/352
// if it gets merged this can be deleted
func CheckEmptyFields(Interface interface{}, result *gojsonschema.Result, schema *jsonschema.Schema) error {
	value := reflect.ValueOf(Interface)
	if value.Kind() != reflect.Ptr {
		return fmt.Errorf("The argument to CheckEmptyFields must be a pointer")
	}
	elem := value.Elem()
	for i := 0; i < elem.NumField(); i++ {
		field := elem.Field(i)
		// if we're dealing with a slice, iterate through
		// it and search for missing required fields in each
		// element of the slice
		if field.Type().Kind() == reflect.Slice {
			for i := 0; i < field.Cap(); i++ {
				sliceElem := field.Index(i)
				if sliceElem.Kind() == reflect.Ptr && sliceElem.Elem().Kind() == reflect.Struct {
					err := CheckEmptyFields(sliceElem.Interface(), result, schema)
					if err != nil {
						return err
					}
				}
			}
		} else if field.Type().Kind() == reflect.Ptr {
			// otherwise if it's just a pointer to a nested struct
			// search it for empty required fields
			if field.Elem().Kind() == reflect.Struct {
				err := CheckEmptyFields(field.Interface(), result, schema)
				if err != nil {
					return err
				}
			}
		} else {

			// check if the field is required and if it is present in the YAML file
			required := false
			tags := elem.Type().Field(i).Tag
			jsonTag, hasJSON := tags.Lookup("json")
			if hasJSON {
				if !strings.Contains(jsonTag, "omitempty") {
					required = true
				}
			}
			// also check for required values in the jsonschema
			for _, requiredField := range schema.Required {
				if elem.Type().Field(i).Name == requiredField {
					required = true
				}
			}
			if required {
				// this is a required field, check for zero values
				if reflect.Indirect(field).IsZero() {
					jsonContext := gojsonschema.NewJsonContext("image_definition", nil)
					errDetail := gojsonschema.ErrorDetails{
						"property": tags.Get("yaml"),
						"parent":   elem.Type().Name(),
					}
					result.AddError(
						newMissingFieldError(
							gojsonschema.NewJsonContext("missing_field", jsonContext),
							52,
							errDetail,
						),
						errDetail,
					)
				}
			}
		}
	}
	return nil
}

func newMissingFieldError(context *gojsonschema.JsonContext, value interface{}, details gojsonschema.ErrorDetails) *MissingFieldError {
	err := MissingFieldError{}
	err.SetContext(context)
	err.SetType("missing_field_error")
	err.SetValue(value)
	err.SetDescriptionFormat("Key \"{{.property}}\" is required in struct \"{{.parent}}\", but is not in the YAML file!")
	err.SetDetails(details)

	return &err
}

// MissingFieldError is used when the fields exist but are the zero value for their type
type MissingFieldError struct {
	gojsonschema.ResultErrorFields
}

// SliceHasElement searches for a string in a slice of strings and returns whether it
// is found
func SliceHasElement(haystack []string, needle string) bool {
	found := false
	for _, element := range haystack {
		if element == needle {
			found = true
		}
	}
	return found
}

// SetCommandOutput sets the output of a command to either use a multiwriter
// or behave as a normal command and store the output in a buffer
func SetCommandOutput(cmd *exec.Cmd, liveOutput bool) (cmdOutput *bytes.Buffer) {
	var cmdOutputBuffer bytes.Buffer
	cmdOutput = &cmdOutputBuffer
	cmd.Stdout = cmdOutput
	cmd.Stderr = cmdOutput
	if liveOutput {
		mwriter := io.MultiWriter(os.Stdout, cmdOutput)
		cmd.Stdout = mwriter
		cmd.Stderr = mwriter
	}
	return cmdOutput
}

// SafeQuantitySubtraction subtracts quantities while checking for integer underflow
func SafeQuantitySubtraction(orig, subtract quantity.Size) quantity.Size {
	if subtract > orig {
		return 0
	}
	return orig - subtract
}

// ExtractTarArchive extracts all the files from a tar. Currently supported are
// uncompressed tar archives and the following compression types: zip, gzip, xz
// bzip2, zstd
func ExtractTarArchive(src, dest string) error {
	// magic numbers are used to determine the compression type of
	// the tar archive
	magicNumbers := map[string][]byte{
		"zip":   {0x50, 0x4b, 0x03, 0x04},
		"gzip":  {0x1f, 0x8b, 0x08},
		"xz":    {0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00},
		"bzip2": {0x42, 0x5a, 0x68},
		"zstd":  {0x28, 0xb5, 0x2f, 0xfd},
	}
	// first check if the archive is gzip compressed
	tarBytes, err := ioutil.ReadFile(src)
	if err != nil {
		return fmt.Errorf("Error reading tar file: \"%s\"", err.Error())
	}

	tarBuff := bytes.NewReader(tarBytes)

	var tarReader *tar.Reader
	found := false
	var fileType string
	for compressionType, magicNumber := range magicNumbers {
		if bytes.HasPrefix(tarBytes, magicNumber) {
			found = true
			fileType = compressionType
		}
	}
	if !found {
		// if it didn't have one of the listed magic numbers, assume it's a tar
		// archive. This will throw an error later in the function if it is not
		// a valid tar archive
		fileType = "tar"
	}
	switch fileType {
	case "zip":
		zipReader, err := zip.NewReader(tarBuff, int64(len(tarBytes)))
		if err != nil {
			return fmt.Errorf("Error reading zip file: \"%s\"", err.Error())
		}
		if len(zipReader.File) > 1 {
			return fmt.Errorf("Invalid zip file. Zip must contain exactly one tar file")
		}
		zipContents, err := zipReader.File[0].Open()
		if err != nil {
			return fmt.Errorf("Error reading contents of zip file: \"%s\"", err.Error())
		}
		tarReader = tar.NewReader(zipContents)
		break
	case "gzip":
		gzipReader, err := gzip.NewReader(tarBuff)
		if err != nil {
			return fmt.Errorf("Error reading gzip file: \"%s\"", err.Error())
		}
		defer gzipReader.Close()
		tarReader = tar.NewReader(gzipReader)
		break
	case "xz":
		xzReader, err := xz.NewReader(tarBuff)
		if err != nil {
			return fmt.Errorf("Error reading xz file: \"%s\"", err.Error())
		}
		tarReader = tar.NewReader(xzReader)
		break
	case "bzip2":
		bzipReader := bzip2.NewReader(tarBuff)
		tarReader = tar.NewReader(bzipReader)
		break
	case "zstd":
		zstdReader, err := zstd.NewReader(tarBuff)
		if err != nil {
			return fmt.Errorf("Error reading zstd file: \"%s\"", err.Error())
		}
		defer zstdReader.Close()
		tarReader = tar.NewReader(zstdReader)
		break
	case "tar":
		// the archive is not compressed, so simply extract it
		tarReader = tar.NewReader(tarBuff)
		break
	}

	for {
		header, err := tarReader.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dest, header.Name)

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file or link create it
		case tar.TypeReg, tar.TypeSymlink, tar.TypeLink:
			destFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(destFile, tarReader); err != nil {
				return err
			}

			// make sure to close the file
			destFile.Close()
		}
	}

	return nil
}

// CalculateSHA256 calculates the SHA256 sum of the file provided as an argument
func CalculateSHA256(fileName string) (string, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return "", fmt.Errorf("Error opening file \"%s\" to calculate SHA256 sum: \"%s\"", fileName, err.Error())
	}
	defer f.Close()

	hasher := sha256.New()
	_, err = io.Copy(hasher, f)
	if err != nil {
		return "", fmt.Errorf("Error calculating SHA256 sum of file \"%s\": \"%s\"", fileName, err.Error())
	}

	return string(hasher.Sum(nil)), nil
}
