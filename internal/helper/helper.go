package helper

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/snapcore/snapd/gadget/quantity"
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

// SetDefaults sets the default values in the struct if they are not
// already set. They are defined though the `default` struct tag.
// Currently only setting strings is supported
/*func SetDefaults(needsDefaults interface{}) {
	indirectedNeedsDefaults := reflect.Indirect(reflect.ValueOf(needsDefaults))
	indirectedInterface := indirectedNeedsDefaults.Interface()
	interfaceType := reflect.TypeOf(indirectedInterface)
	interfaceFields := reflect.VisibleFields(interfaceType)
	for _, field := range interfaceFields {
		// check types and dereference pointers
		var fieldKind reflect.Kind
		var nonPointerValue reflect.Value
		if field.Type.Kind() == reflect.Ptr {
			fieldPointer := reflect.ValueOf(indirectedInterface).FieldByIndex(field.Index)
			if !fieldPointer.IsNil() {
				indirectedField := reflect.Indirect(fieldPointer)
				fieldKind = indirectedField.Type().Kind()
				nonPointerValue = indirectedField
			}
		} else {
			fieldKind = field.Type.Kind()
			nonPointerValue = reflect.ValueOf(indirectedInterface).FieldByIndex(field.Index)
		}

		// if the current field is a non-nil struct, call this function recursively
		if fieldKind == reflect.Struct {
			nonPointerInterface := nonPointerValue.Interface()
			fmt.Println(reflect.TypeOf(nonPointerInterface))
			SetDefaults(&nonPointerInterface)
		}
		// otherwise, look for the "default" struct tag
		defaultValue, hasDefault := field.Tag.Lookup("default")
		if hasDefault {
			fmt.Printf("Key %s of type %s has default value %s\n", field.Name, field.Type, defaultValue)
			addressableValue := nonPointerValue.String()
			fmt.Println(reflect.TypeOf(addressableValue))
			if addressableValue.CanAddr() {
				fmt.Println("JAWN this field is addressable")
			} else {
				fmt.Println("JAWN this field is NOT addressable")
			}
			if nonPointerValue.CanSet() {
				fmt.Println("JAWN we can set the value!")
				nonPointerValue.SetString(defaultValue)
			}
		}
	}
}*/
func SetDefaults(needsDefaults interface{}) {
	value := reflect.ValueOf(needsDefaults)
	if value.Kind() != reflect.Ptr {
		panic("The argument to SetDefaults must be a pointer!")
	}
	elem := value.Elem()
	for i := 0; i < elem.NumField(); i++ {
		field := elem.Field(i)
		// no sense checking non-pointer values as they can't be updated
		if field.Type().Kind() == reflect.Ptr {
			if field.Elem().Kind() == reflect.Struct {
				SetDefaults(field.Interface())
			}
		}
		tags := elem.Type().Field(i).Tag
		defaultValue, hasDefault := tags.Lookup("default")
		if hasDefault {
			fmt.Printf(defaultValue)
			fmt.Println(field.Type())
			fmt.Println(field.Type().Kind())
			indirectedField := reflect.Indirect(field)
			if indirectedField.CanSet() {
				fmt.Println("JAWN setting default value")
				field.SetString(defaultValue)
			} else {
				fmt.Println("JAWN cannot set default value")
			}
		}
	}
}

/*func setField(field reflect.Value, defaultVal string) error {

	if !field.CanSet() {
		return fmt.Errorf("Can't set value\n")
	}

	switch field.Kind() {

	case reflect.Int:
		if val, err := strconv.ParseInt(defaultVal, 10, 64); err == nil {
			field.Set(reflect.ValueOf(int(val)).Convert(field.Type()))
		}
	case reflect.String:
		field.Set(reflect.ValueOf(defaultVal).Convert(field.Type()))
	}

	return nil
}

func Set(ptr interface{}, tag string) error {
	if reflect.TypeOf(ptr).Kind() != reflect.Ptr {
		return fmt.Errorf("Not a pointer")
	}

	v := reflect.ValueOf(ptr).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		if defaultVal := t.Field(i).Tag.Get(tag); defaultVal != "-" {
			if err := setField(v.Field(i), defaultVal); err != nil {
				return err
			}

		}
	}
	return nil
}*/
