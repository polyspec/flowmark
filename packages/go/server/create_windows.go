//go:build windows

package server

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// createOwnerOnly creates the new file p for writing with a protected access list that grants the user of this process
// alone full access. The permission bits of os.OpenFile set only the read-only attribute on Windows, so a file they
// create inherits the access list of its folder (T15-37).
func createOwnerOnly(p string) (*os.File, error) {
	sddl, err := ownerOnlySDDL()
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: p, Err: err}
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, &sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: p, Err: err}
	}
	return os.NewFile(uintptr(h), p), nil
}

// ownerOnlySDDL is the security descriptor of an owner-only file: a protected access list, so nothing is inherited, with
// one entry granting full access to the user of this process.
func ownerOnlySDDL() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return "D:P(A;;FA;;;" + u.User.Sid.String() + ")", nil
}
