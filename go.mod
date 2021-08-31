module github.com/canonical/ubuntu-image

go 1.16

require (
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/canonical/go-sp800.90a-drbg v0.0.0-20210314144037-6eeb1040d6c3 // indirect
	github.com/canonical/go-tpm2 v0.0.0-20210630093425-c4bb37200aa6 // indirect
	github.com/canonical/tcglog-parser v0.0.0-20200908165021-12a3a7bcf5a1 // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/jessevdk/go-flags v1.5.0
	github.com/juju/ratelimit v1.0.1 // indirect
	github.com/mvo5/goconfigparser v0.0.0-20201015074339-50f22f44deb5 // indirect
	github.com/snapcore/bolt v1.3.1 // indirect
	github.com/snapcore/go-gettext v0.0.0-20201130093759-38740d1bd3d2 // indirect
	github.com/snapcore/secboot v0.0.0-20210805184555-c9f2139ee92b // indirect
	github.com/snapcore/snapd v0.0.0-20210824074111-a3fe6b7e2554
	github.com/snapcore/snapd/osutil/udev v0.0.0-00010101000000-000000000000 // indirect
	github.com/snapcore/squashfuse v0.0.0-20171220165323-319f6d41a041 // indirect
	go.mozilla.org/pkcs7 v0.0.0-20200128120323-432b2356ecb1 // indirect
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97 // indirect
	golang.org/x/net v0.0.0-20210405180319-a5a99cb37ef4 // indirect
	gopkg.in/macaroon.v1 v1.0.0 // indirect
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22 // indirect
	gopkg.in/retry.v1 v1.0.3 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	maze.io/x/crypto v0.0.0-20190131090603-9b94c9afe066 // indirect
)

replace github.com/snapcore/snapd/osutil/udev => github.com/pilebones/go-udev v0.0.0-20210126000448-a3c2a7a4afb7
