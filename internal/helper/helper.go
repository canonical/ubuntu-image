package helper

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"
	"github.com/xeipuuv/gojsonschema"

	"github.com/canonical/ubuntu-image/internal/commands"
)

// define some functions that can be mocked by test cases
var (
	osRename         = os.Rename
	osRemove         = os.Remove
	osWriteFile      = os.WriteFile
	osutilFileExists = osutil.FileExists
)

func BoolPtr(b bool) *bool {
	return &b
}

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
		if isSliceOfPtrToStructs(field) {
			err := setDefaultsToSlice(field)
			if err != nil {
				return err
			}
		} else if field.Type().Kind() == reflect.Ptr {
			err := setDefaultToPtr(field, elem, i)
			if err != nil {
				return err
			}
		} else {
			err := setDefaultToBasicType(field, elem, i)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// setDefaultsToSlice sets default values to elements of a slice.
// It assumes it was already checked field is a non empty slice. Otherwise this
// function will probably panic.
func setDefaultsToSlice(field reflect.Value) error {
	for i := 0; i < field.Cap(); i++ {
		err := SetDefaults(field.Index(i).Interface())
		if err != nil {
			return err
		}
	}
	return nil
}

// setDefaultToPtr sets a default value to a field being a pointer.
// It assumes it was already checked that field is a ptr. Otherwise this
// function will probably panic.
func setDefaultToPtr(field reflect.Value, elem reflect.Value, fieldIndex int) error {
	// if it's a pointer to a struct, look for default types
	if field.Elem().Kind() == reflect.Struct {
		err := SetDefaults(field.Interface())
		if err != nil {
			return err
		}
		// special case for pointer to bools
	} else if field.Type().Elem() == reflect.TypeOf(true) {
		// if a value is set, do nothing
		if !field.IsNil() {
			return nil
		}
		tags := elem.Type().Field(fieldIndex).Tag
		defaultValue, hasDefault := tags.Lookup("default")
		if !hasDefault {
			// If no default and no value is set, make sure we have a valid
			// value consistent with the "zero" value for a bool (false)
			field.Set(reflect.ValueOf(BoolPtr(false)))
			return nil
		}
		if defaultValue == "true" {
			field.Set(reflect.ValueOf(BoolPtr(true)))
		} else {
			field.Set(reflect.ValueOf(BoolPtr(false)))
		}
	}
	return nil
}

// setDefaultToBasicType sets the default value to a basic type (and slice) based on the
// "default" tag.
func setDefaultToBasicType(field reflect.Value, elem reflect.Value, fieldIndex int) error {
	tags := elem.Type().Field(fieldIndex).Tag
	defaultValue, hasDefault := tags.Lookup("default")
	if !hasDefault {
		return nil
	}
	indirectedField := reflect.Indirect(field)
	if indirectedField.CanSet() && field.IsZero() {
		varType := field.Type().Kind()
		switch varType {
		case reflect.String:
			field.SetString(defaultValue)
		case reflect.Slice:
			defaultValues := strings.Split(defaultValue, ",")
			field.Set(reflect.ValueOf(defaultValues))
		case reflect.Bool:
			return fmt.Errorf("Setting default value of a boolean not supported. Use a pointer to boolean instead.")
		default:
			return fmt.Errorf("Setting default value of type %s not supported",
				varType)
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
			err := checkEmptyFieldsInSlice(field, result, schema)
			if err != nil {
				return err
			}
		} else if field.Type().Kind() == reflect.Ptr {
			// otherwise if it's just a pointer to a nested struct
			// search it for empty required fields
			err := checkEmptyFieldsInPtr(field, result, schema)
			if err != nil {
				return err
			}
		} else {
			tags := elem.Type().Field(i).Tag
			if !isRequiredFromTags(tags) && !isRequiredFromSchema(elem, i, schema) {
				continue
			}
			// this is a required field, check for zero values
			if !reflect.Indirect(field).IsZero() {
				continue
			}
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
	return nil
}

func checkEmptyFieldsInSlice(field reflect.Value, result *gojsonschema.Result, schema *jsonschema.Schema) error {
	for i := 0; i < field.Cap(); i++ {
		sliceElem := field.Index(i)
		if sliceElem.Kind() == reflect.Ptr && sliceElem.Elem().Kind() == reflect.Struct {
			err := CheckEmptyFields(sliceElem.Interface(), result, schema)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func checkEmptyFieldsInPtr(field reflect.Value, result *gojsonschema.Result, schema *jsonschema.Schema) error {
	if field.Elem().Kind() == reflect.Struct {
		err := CheckEmptyFields(field.Interface(), result, schema)
		if err != nil {
			return err
		}
	}
	return nil
}

// isRequiredFromTags checks if the field is required from the JSON tags
func isRequiredFromTags(tags reflect.StructTag) bool {
	jsonTag, hasJSON := tags.Lookup("json")
	if hasJSON {
		if !strings.Contains(jsonTag, "omitempty") {
			return true
		}
	}
	return false
}

// isRequiredFromSchema checks if the field is required from the schema
func isRequiredFromSchema(elem reflect.Value, i int, schema *jsonschema.Schema) bool {
	for _, requiredField := range schema.Required {
		if elem.Type().Field(i).Name == requiredField {
			return true
		}
	}
	return false
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

func RunCmd(cmd *exec.Cmd, debug bool) error {
	output := SetCommandOutput(cmd, debug)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Error running command \"%s\". Error: %s. Output:\n%s",
			cmd.String(), err.Error(), output.String())
	}
	return nil
}

// RunCmds runs a list of commands and returns the error
// It stops at the first error
func RunCmds(cmds []*exec.Cmd, debug bool) error {
	for _, cmd := range cmds {
		err := RunCmd(cmd, debug)
		if err != nil {
			return err
		}
	}

	return nil
}

// SafeQuantitySubtraction subtracts quantities while checking for integer underflow
func SafeQuantitySubtraction(orig, subtract quantity.Size) quantity.Size {
	if subtract > orig {
		return 0
	}
	return orig - subtract
}

// CreateTarArchive places all of the files from a source directory into a tar.
// Currently supported are uncompressed tar archives and the following
// compression types: gzip, xz bzip2, zstd
func CreateTarArchive(src, dest, compression string, debug bool) error {
	tarCommand := exec.Command(
		"tar",
		"--directory",
		src,
		"--xattrs",
		"--xattrs-include=*",
		"--sparse",
		"--create",
		"--file",
		dest,
		".",
	)
	if debug {
		tarCommand.Args = append(tarCommand.Args, "--verbose")
	}
	// set up any compression arguments
	switch compression {
	case "uncompressed":
		break
	case "bzip2":
		tarCommand.Args = append(tarCommand.Args, "--bzip2")
	case "gzip":
		tarCommand.Args = append(tarCommand.Args, "--gzip")
	case "xz":
		tarCommand.Args = append(tarCommand.Args, "--xz")
	case "zstd":
		tarCommand.Args = append(tarCommand.Args, "--zstd")
	default:
		return fmt.Errorf("Unknown compression type: \"%s\"", compression)
	}
	return RunCmd(tarCommand, debug)
}

// ExtractTarArchive extracts all the files from a tar. Currently supported are
// uncompressed tar archives and the following compression types: zip, gzip, xz
// bzip2, zstd
func ExtractTarArchive(src, dest string, debug bool) error {
	tarCommand := exec.Command(
		"tar",
		"--xattrs",
		"--xattrs-include=*",
		"--extract",
		"--file",
		src,
		"--directory",
		dest,
	)
	if debug {
		tarCommand.Args = append(tarCommand.Args, "--verbose")
	}
	return RunCmd(tarCommand, debug)
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

// CheckTags iterates through the keys in a struct and looks for
// a value passed in as a parameter. It returns the yaml name of
// the key and an error. Currently only boolean values for the tags
// are supported
func CheckTags(searchStruct interface{}, tag string) (string, error) {
	value := reflect.ValueOf(searchStruct)
	if value.Kind() != reflect.Ptr {
		return "", fmt.Errorf("The argument to CheckTags must be a pointer")
	}
	elem := value.Elem()

	return checkTagsOnField(elem, tag)
}

func isSliceOfPtrToStructs(field reflect.Value) bool {
	return field.Type().Kind() == reflect.Slice &&
		field.Cap() > 0 &&
		field.Index(0).Kind() == reflect.Pointer
}

func checkTagsOnField(elem reflect.Value, tag string) (string, error) {
	for i := 0; i < elem.NumField(); i++ {
		field := elem.Field(i)
		// if we're dealing with a slice of pointers to structs,
		// iterate through it and check the tags for each struct pointer
		if isSliceOfPtrToStructs(field) {
			for i := 0; i < field.Cap(); i++ {
				tagUsed, err := CheckTags(field.Index(i).Interface(), tag)
				if err != nil {
					return "", err
				}
				if tagUsed != "" {
					// just return on the first one found.
					// user can iteratively work through error
					// messages if there are more than one
					return tagUsed, nil
				}
			}
		} else if !field.IsNil() {
			tags := elem.Type().Field(i).Tag
			tagValue, hasTag := tags.Lookup(tag)
			if hasTag && tagValue == "true" {
				yamlName, _ := tags.Lookup("yaml")
				return yamlName, nil
			}
		}
	}
	// no true value found
	return "", nil
}

// BackupAndCopyResolvConf creates a backup of /etc/resolv.conf in a chroot
// and copies the contents from the host system into the chroot
func BackupAndCopyResolvConf(chroot string) error {
	if osutil.FileExists(filepath.Join(chroot, "etc", "resolv.conf.tmp")) {
		// already backed up/copied so do nothing
		return nil
	}
	src := filepath.Join(chroot, "etc", "resolv.conf")
	dest := filepath.Join(chroot, "etc", "resolv.conf.tmp")
	if err := os.Rename(src, dest); err != nil {
		return fmt.Errorf("Error moving file \"%s\" to \"%s\": %s", src, dest, err.Error())
	}
	dest = src
	src = filepath.Join("/etc", "resolv.conf")
	if err := osutil.CopyFile(src, dest, osutil.CopyFlagDefault); err != nil {
		return fmt.Errorf("Error copying file \"%s\" to \"%s\": %s", src, dest, err.Error())
	}
	return nil
}

// RestoreResolvConf restores the resolv.conf in the chroot from the
// version that was backed up by BackupAndCopyResolvConf
func RestoreResolvConf(chroot string) error {
	if !osutil.FileExists(filepath.Join(chroot, "etc", "resolv.conf.tmp")) {
		return nil
	}
	if osutil.IsSymlink(filepath.Join(chroot, "etc", "resolv.conf")) {
		// As per what live-build does, handle the case where some package
		// in the install_packages phase converts resolv.conf into a
		// symlink. In such case we don't restore our backup but instead
		// remove it, leaving the symlink around.
		backup := filepath.Join(chroot, "etc", "resolv.conf.tmp")
		if err := osRemove(backup); err != nil {
			return fmt.Errorf("Error removing file \"%s\": %s", backup, err.Error())
		}
	} else {
		src := filepath.Join(chroot, "etc", "resolv.conf.tmp")
		dest := filepath.Join(chroot, "etc", "resolv.conf")
		if err := osRename(src, dest); err != nil {
			return fmt.Errorf("Error moving file \"%s\" to \"%s\": %s", src, dest, err.Error())
		}
	}
	return nil
}

const backupExt = ".REAL"

// BackupReplace backup the target file and replace it with the given content
// Returns the restore function.
func BackupReplace(target string, content string) (func(error) error, error) {
	backup := target + backupExt
	if osutilFileExists(backup) {
		// already backed up so do nothing
		return nil, nil
	}

	if err := osRename(target, backup); err != nil {
		return nil, fmt.Errorf("Error moving file \"%s\" to \"%s\": %s", target, backup, err.Error())
	}

	if err := osWriteFile(target, []byte(content), 0755); err != nil {
		return nil, fmt.Errorf("Error writing to %s : %s", target, err.Error())
	}

	return genRestoreFile(target), nil
}

// genRestoreFile returns the function to be called to restore the backuped file
func genRestoreFile(target string) func(err error) error {
	return func(err error) error {
		src := target + backupExt
		if !osutilFileExists(src) {
			return err
		}

		if tmpErr := osRename(src, target); tmpErr != nil {
			tmpErr = fmt.Errorf("Error moving file \"%s\" to \"%s\": %s", src, target, tmpErr.Error())
			return fmt.Errorf("%s\n%s", err, tmpErr)
		}

		return err
	}
}
