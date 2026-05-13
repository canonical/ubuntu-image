package commands

// M2cpCLI is the name of the appstore CLI binary that ubuntu-image
// shells out to for trust bootstrap, model push, snap-info lookups,
// download-URL minting, and assertion fetches.
//
// The binary is scheduled to be renamed in the future; every
// exec.Command / exec.LookPath / human-facing message that
// references it goes through this constant so the rename is a
// one-line change.
const M2cpCLI = "m2cp"
