package statemachine

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/canonical/ubuntu-image/internal/helper"
)

func Test_getMountCmd(t *testing.T) {
	tests := []struct {
		name           string
		mp             mountPoint
		wantMountCmds  []string
		wantUmountCmds []string
		expectedError  string
	}{
		{
			name: "happy path",
			mp: mountPoint{
				src:      "src",
				basePath: "targetDir",
				relpath:  "mountpoint",
				typ:      "devtmps",
				opts:     []string{"nodev", "nosuid"},
			},
			wantMountCmds: []string{"/usr/bin/mount -t devtmps src -o nodev,nosuid targetDir/mountpoint"},
			wantUmountCmds: []string{
				"/usr/bin/mount --make-rprivate targetDir/mountpoint",
				"/usr/bin/umount --recursive targetDir/mountpoint",
			},
		},
		{
			name: "no type",
			mp: mountPoint{
				src:      "src",
				basePath: "targetDir",
				relpath:  "mountpoint",
				typ:      "",
			},
			wantMountCmds: []string{"/usr/bin/mount src targetDir/mountpoint"},
			wantUmountCmds: []string{
				"/usr/bin/mount --make-rprivate targetDir/mountpoint",
				"/usr/bin/umount --recursive targetDir/mountpoint",
			},
		},
		{
			name: "bind mount",
			mp: mountPoint{
				src:      "src",
				basePath: "targetDir",
				relpath:  "mountpoint",
				typ:      "",
				bind:     true,
			},
			wantMountCmds: []string{"/usr/bin/mount --bind src targetDir/mountpoint"},
			wantUmountCmds: []string{
				"/usr/bin/mount --make-rprivate targetDir/mountpoint",
				"/usr/bin/umount --recursive targetDir/mountpoint",
			},
		},
		{
			name: "no src",
			mp: mountPoint{
				src:      "",
				basePath: "targetDir",
				relpath:  "mountpoint",
				typ:      "",
				bind:     true,
			},
			wantMountCmds: []string{"/usr/bin/mount --bind  targetDir/mountpoint"},
			wantUmountCmds: []string{
				"/usr/bin/mount --make-rprivate targetDir/mountpoint",
				"/usr/bin/umount --recursive targetDir/mountpoint",
			},
		},
		{
			name: "fail with bind and type",
			mp: mountPoint{
				src:      "src",
				basePath: "targetDir",
				relpath:  "mountpoint",
				typ:      "devtmps",
				bind:     true,
			},
			wantMountCmds:  []string{},
			wantUmountCmds: []string{},
			expectedError:  "invalid mount arguments. Cannot use --bind and -t at the same time.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			gotMountCmds, gotUmountCmds, err := tc.mp.getMountCmd()

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

	mp := mountPoint{
		typ:      "devtmps",
		basePath: "/tmp",
		relpath:  "1234567",
		src:      "src",
	}

	gotMountCmds, gotUmountCmds, err := mp.getMountCmd()
	asserter.AssertErrContains(err, "Error creating mountpoint")
	if gotMountCmds != nil {
		asserter.Errorf("gotMountCmds should be nil but is %s", gotMountCmds)
	}
	if gotUmountCmds != nil {
		asserter.Errorf("gotUmountCmds should be nil but is %s", gotUmountCmds)
	}
}

var (
	mp1 = mountPoint{
		src:      "srcmp1",
		path:     "src1basePath/src1relpath",
		basePath: "src1basePath",
		relpath:  "src1relpath",
		typ:      "devtmpfs",
	}
	mp2 = mountPoint{
		src:      "srcmp2",
		path:     "src2basePath/src2relpath",
		basePath: "src2basePath",
		relpath:  "src2relpath",
		typ:      "devpts",
	}
	mp3 = mountPoint{
		src:      "srcmp3",
		path:     "src3basePath/src3relpath",
		basePath: "src3basePath",
		relpath:  "src3relpath",
		typ:      "proc",
	}
	mp4 = mountPoint{
		src:      "srcmp4",
		path:     "src4basePath/src4relpath",
		basePath: "src4basePath",
		relpath:  "src4relpath",
		typ:      "cgroup2",
	}
	mp11 = mountPoint{
		src:      "srcmp12",
		path:     "src1basePath/src1relpath",
		basePath: "src1basePath",
		relpath:  "src1relpath",
		typ:      "devtmpfs",
	}
	mp21 = mountPoint{
		src:      "srcmp2",
		path:     "",
		basePath: "src21basePath",
		relpath:  "src2relpath",
		typ:      "devpts",
	}
	mp31 = mountPoint{
		src:      "srcmp3",
		path:     "",
		basePath: "src3basePath",
		relpath:  "src31relpath",
		typ:      "proc",
	}
	mp41 = mountPoint{
		src:      "srcmp4",
		path:     "src4basePath/src4relpath",
		basePath: "src4basePath",
		relpath:  "src4relpath",
		typ:      "anotherType",
	}
)

func Test_diffMountPoints(t *testing.T) {
	asserter := helper.Asserter{T: t}
	type args struct {
		olds     []*mountPoint
		currents []*mountPoint
	}

	cmpOpts := []cmp.Option{
		cmp.AllowUnexported(
			mountPoint{},
		),
	}

	tests := []struct {
		name string
		args args
		want []*mountPoint
	}{
		{
			name: "same mounpoints, ignoring list order",
			args: args{
				olds: []*mountPoint{
					&mp1,
					&mp2,
					&mp3,
					&mp4,
				},
				currents: []*mountPoint{
					&mp4,
					&mp1,
					&mp3,
					&mp2,
				},
			},
			want: nil,
		},
		{
			name: "add some",
			args: args{
				olds: []*mountPoint{
					&mp1,
					&mp2,
				},
				currents: []*mountPoint{
					&mp3,
					&mp4,
				},
			},
			want: []*mountPoint{
				&mp3,
				&mp4,
			},
		},
		{
			name: "no old ones",
			args: args{
				olds: nil,
				currents: []*mountPoint{
					&mp3,
					&mp4,
				},
			},
			want: []*mountPoint{
				&mp3,
				&mp4,
			},
		},
		{
			name: "no current ones",
			args: args{
				olds: []*mountPoint{
					&mp1,
					&mp2,
				},
				currents: nil,
			},
			want: nil,
		},
		{
			name: "difference in src, relpath, basepath and typ",
			args: args{
				olds: []*mountPoint{
					&mp1,
					&mp2,
					&mp3,
					&mp4,
				},
				currents: []*mountPoint{
					&mp11,
					&mp21,
					&mp31,
					&mp41,
				},
			},
			want: []*mountPoint{
				&mp11,
				&mp21,
				&mp31,
				&mp41,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffMountPoints(tt.args.olds, tt.args.currents)
			asserter.AssertEqual(tt.want, got, cmpOpts...)
		})
	}
}
