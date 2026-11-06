//go:build windows

package journal

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/polyspec/flowmark/packages/go/internal/cost"
)

// currentUser returns the security identifier of this process's user.
func currentUser() (*windows.SID, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("the user of this process: %w", err)
	}
	return u.User.Sid, nil
}

// mkdirPrivate creates the folder p with a protected access list that grants this process's user alone, inherited by
// what is created in it. Windows cannot flush a folder; the folder holds only the databases, which a crash leaves for
// the recovery.
func mkdirPrivate(_ *cost.Counts, p string) error {
	sid, err := currentUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid.String() + ")")
	if err != nil {
		return fmt.Errorf("the access list of %s: %w", p, err)
	}
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return err
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if err := windows.CreateDirectory(name, sa); err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return &os.PathError{Op: "mkdir", Path: p, Err: fs.ErrExist}
		}
		return &os.PathError{Op: "mkdir", Path: p, Err: err}
	}
	return nil
}

// checkPrivate refuses the folder p unless its access list is protected from inheritance and grants this process's user
// alone.
func checkPrivate(p string) error {
	sid, err := currentUser()
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("the access list of %s: %w", p, err)
	}
	control, _, err := sd.Control()
	if err != nil {
		return fmt.Errorf("the access list of %s: %w", p, err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("%s inherits its access list; the databases' folder is its owner's alone", p)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("the access list of %s: %w", p, err)
	}
	if dacl == nil {
		return fmt.Errorf("%s has no access list, which grants everyone; the databases' folder is its owner's alone", p)
	}
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			return fmt.Errorf("the access list of %s: %w", p, err)
		}
		switch ace.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			if granted := (*windows.SID)(unsafe.Pointer(&ace.SidStart)); !granted.Equals(sid) {
				return fmt.Errorf("%s grants access to %s, not this process's user %s alone; the databases' folder is its owner's alone", p, granted, sid)
			}
		default:
			return fmt.Errorf("%s holds an access entry of type %d, which flowmark does not judge; the databases' folder is its owner's alone", p, ace.Header.AceType)
		}
	}
	return nil
}
