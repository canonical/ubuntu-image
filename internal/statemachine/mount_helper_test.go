package statemachine

import (
	"os"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
)

func Test_getMountCmd(t *testing.T) {
	type args struct {
		typ        string
		src        string
		targetDir  string
		mountpoint string
		bind       bool
		options    []string
	}
	tests := []struct {
		name           string
		args           args
		wantMountCmds  []string
		wantUmountCmds []string
		expectedError  string
	}{
		{
			name: "happy path",
			args: args{
				typ:        "devtmps",
				src:        "src",
				targetDir:  "targetDir",
				mountpoint: "mountpoint",
				options:    []string{"nodev", "nosuid"},
			},
			wantMountCmds: []string{"/usr/bin/mount -t devtmps src -o nodev,nosuid targetDir/mountpoint"},
			wantUmountCmds: []string{
				"/usr/bin/mount --make-rprivate targetDir/mountpoint",
				"/usr/bin/umount --recursive targetDir/mountpoint",
			},
		},
		{
			name: "no type",
			args: args{
				typ:        "",
				src:        "src",
				targetDir:  "targetDir",
				mountpoint: "mountpoint",
			},
			wantMountCmds: []string{"/usr/bin/mount src targetDir/mountpoint"},
			wantUmountCmds: []string{
				"/usr/bin/mount --make-rprivate targetDir/mountpoint",
				"/usr/bin/umount --recursive targetDir/mountpoint",
			},
		},
		{
			name: "bind mount",
			args: args{
				typ:        "",
				src:        "src",
				targetDir:  "targetDir",
				mountpoint: "mountpoint",
				bind:       true,
			},
			wantMountCmds: []string{"/usr/bin/mount --bind src targetDir/mountpoint"},
			wantUmountCmds: []string{
				"/usr/bin/mount --make-rprivate targetDir/mountpoint",
				"/usr/bin/umount --recursive targetDir/mountpoint",
			},
		},
		{
			name: "fail with bind and type",
			args: args{
				typ:        "devtmps",
				src:        "src",
				targetDir:  "targetDir",
				mountpoint: "mountpoint",
				bind:       true,
			},
			wantMountCmds:  []string{},
			wantUmountCmds: []string{},
			expectedError:  "invalid mount arguments. Cannot use --bind and -t at the same time.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			gotMountCmds, gotUmountCmds, err := getMountCmd(tc.args.typ, tc.args.src, tc.args.targetDir, tc.args.mountpoint, tc.args.bind, tc.args.options...)

			if len(tc.expectedError) == 0 {
				asserter.AssertErrNil(err, true)
			} else {
				asserter.AssertErrContains(err, tc.expectedError)
			}

			gotMountCmdsStr := make([]string, 0)
			gotUmountCmdsStr := make([]string, 0)

			for _, c := range gotMountCmds {
				gotMountCmdsStr = append(gotMountCmdsStr, c.String())
			}

			for _, c := range gotUmountCmds {
				gotUmountCmdsStr = append(gotUmountCmdsStr, c.String())
			}
			asserter.AssertEqual(tc.wantMountCmds, gotMountCmdsStr)
			asserter.AssertEqual(tc.wantUmountCmds, gotUmountCmdsStr)

		})
	}
}

func Test_getMountCmd_fail(t *testing.T) {
	asserter := helper.Asserter{T: t}

	// mock os.Mkdir
	osMkdirAll = mockMkdirAll
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	gotMountCmds, gotUmountCmds, err := getMountCmd("devtmps", "src", "/tmp", "1234567", false)
	asserter.AssertErrContains(err, "Error creating mountpoint")
	if gotMountCmds != nil {
		asserter.Errorf("gotMountCmds should be nil but is %s", gotMountCmds)
	}
	if gotUmountCmds != nil {
		asserter.Errorf("gotUmountCmds should be nil but is %s", gotUmountCmds)
	}
}
