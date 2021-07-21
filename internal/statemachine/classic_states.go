package statemachine

import (
	"fmt"

	//github.com/snapcore/snap
)

// Prepare the gadget tree
func (stateMachine *StateMachine) prepareGadgetTree() error {
        /*gadget_dir = stateMachine.tempDirs.unpack + "/gadget"
        shutil.copytree(self.gadget_tree, gadget_dir)
        # We assume the gadget tree was built from a gadget source tree using
        # snapcraft prime so the gadget.yaml file is expected in the meta/
        # directory.
        self.yaml_file_path = os.path.join(
            gadget_dir, 'meta', 'gadget.yaml')*/
	fmt.Println("Doing prepareGadgetTree, this state is only in classic builds")
	return nil
}

func (stateMachine *StateMachine) prepareImageClassic() error {
        /*if not self.args.filesystem:
            try:
                # Configure it with environment variables.
                env = {}
                if self.args.project is not None:
                    env['PROJECT'] = self.args.project
                if self.args.suite is not None:
                    env['SUITE'] = self.args.suite
                if self.args.arch is not None:
                    env['ARCH'] = self.args.arch
                if self.args.subproject is not None:
                    env['SUBPROJECT'] = self.args.subproject
                if self.args.subarch is not None:
                    env['SUBARCH'] = self.args.subarch
                if self.args.with_proposed:
                    env['PROPOSED'] = '1'
                if self.args.extra_ppas is not None:
                    env['EXTRA_PPAS'] = ' '.join(self.args.extra_ppas)
                # Only generate a single rootfs tree for classic images.
                env['IMAGEFORMAT'] = 'none'
                # ensure ARCH is set
                if self.args.arch is None:
                    env['ARCH'] = get_host_arch()
                live_build(self.unpackdir, env)
            except CalledProcessError:
                if self.args.debug:
                    _logger.exception('Full debug traceback follows')
                self.exitcode = 1
                # Stop the state machine here by not appending a next step.*/
	fmt.Println("Doing image preparation specific to classic")
	return nil
}
