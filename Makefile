.PHONY: snap
snap:  ## Create a ubuntu-image snap
	@snapcraft clean && snapcraft --use-lxd -v

.PHONY: collect-mkfs-confs
collect-mkfs-confs:
	@./tools/collect-mkfs-confs.sh
