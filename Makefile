.PHONY: snap
snap:  ## Create a ubuntu-image snap
	@snapcraft clean && snapcraft --use-lxd -v
