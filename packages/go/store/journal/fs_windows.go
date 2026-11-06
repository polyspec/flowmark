//go:build windows

package journal

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"

	"golang.org/x/sys/windows"
)

// stageFoldersDefault: Windows has no folder flush, so a created folder is made in the stage and moved into place
// with a write-through move (spec 40, durability).
const stageFoldersDefault = true

const shareAll = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE

// openShared opens a file of the journal or the lock with CreateFileW sharing read, write and delete (REVIEW 1-d), so
// a reader never blocks the holder's delete or an unlock, and opens a reparse point itself instead of following it.
func openShared(p string, flag int, perm os.FileMode) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: p, Err: err}
	}
	var access uint32
	switch flag & (os.O_RDONLY | os.O_WRONLY | os.O_RDWR) {
	case os.O_RDONLY:
		access = windows.GENERIC_READ
	case os.O_WRONLY:
		access = windows.GENERIC_WRITE
	default:
		access = windows.GENERIC_READ | windows.GENERIC_WRITE
	}
	var disposition uint32
	switch {
	case flag&(os.O_CREATE|os.O_EXCL) == os.O_CREATE|os.O_EXCL:
		disposition = windows.CREATE_NEW
	case flag&(os.O_CREATE|os.O_TRUNC) == os.O_CREATE|os.O_TRUNC:
		disposition = windows.CREATE_ALWAYS
	case flag&os.O_CREATE == os.O_CREATE:
		disposition = windows.OPEN_ALWAYS
	case flag&os.O_TRUNC == os.O_TRUNC:
		disposition = windows.TRUNCATE_EXISTING
	default:
		disposition = windows.OPEN_EXISTING
	}
	var h windows.Handle
	pending, err := deletePending(func() (err error) {
		h, err = windows.CreateFile(name, access, shareAll, nil, disposition, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		return err
	})
	if err != nil {
		if pending {
			// The name is removed as far as its remover is concerned: an open finds no file, an exclusive create one.
			is := fs.ErrNotExist
			if disposition == windows.CREATE_NEW {
				is = fs.ErrExist
			}
			err = &pendingDelete{err: err, is: is}
		}
		return nil, &os.PathError{Op: "open", Path: p, Err: err}
	}
	return os.NewFile(uintptr(h), p), nil
}

// fileIdentity is not used on Windows: the wait watches the folder, not the lock file.
func fileIdentity(*os.File) (fileID, error) { return fileID{}, nil }

// moveDurable moves src to dst with MoveFileEx and MOVEFILE_WRITE_THROUGH, which returns once the move is on disk;
// Windows has no folder flush, so this is how a name is made durable there.
func moveDurable(src, dst string) error {
	return moveFileEx(src, dst, windows.MOVEFILE_REPLACE_EXISTING|writeThrough())
}

// moveFolderDurable moves a folder to a name that does not exist; MOVEFILE_REPLACE_EXISTING cannot be used for a
// folder.
func moveFolderDurable(src, dst string) error {
	return moveFileEx(src, dst, writeThrough())
}

// writeThrough is MOVEFILE_WRITE_THROUGH, unless this process does not wait for its writes to reach the disk
// (SetSynchronous), as syncDir and writeSynced do not on the other platforms.
func writeThrough() uint32 {
	if asynchronous.Load() {
		return 0
	}
	return windows.MOVEFILE_WRITE_THROUGH
}

func moveFileEx(src, dst string, flags uint32) error {
	from, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(from, to, flags); err != nil {
		return &os.LinkError{Op: "move", Old: src, New: dst, Err: err}
	}
	return nil
}

// mkdirDurable creates one of the journal's own folders (`.flowmark/`, `.flowmark/tx/`, a stage). Windows cannot
// flush a folder; these folders hold only files moved in with write-through moves, and a corpus folder is created in
// the stage and moved into place instead.
func mkdirDurable(p string) error { return os.Mkdir(p, 0o755) }

// syncRemoved does nothing on Windows, which has no folder flush. It follows only the removal of a collected or
// discarded stage, which a power loss may bring back; recovery then collects or discards it again.
func syncRemoved(string) error { return nil }

// deviceOf returns the volume serial number of a path, from GetFileInformationByHandle.
func deviceOf(p string) (uint64, error) {
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(name, 0, shareAll, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, &os.PathError{Op: "open", Path: p, Err: err}
	}
	var info windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(h, &info)
	if err := errors.Join(err, windows.CloseHandle(h)); err != nil {
		return 0, fmt.Errorf("the volume of %s: %w", p, err)
	}
	return uint64(info.VolumeSerialNumber), nil
}

// linkUnsupported reports a link refused because the file system has no hard links (FAT32, exFAT).
func linkUnsupported(err error) bool {
	return errors.Is(err, windows.ERROR_INVALID_FUNCTION) || errors.Is(err, windows.ERROR_NOT_SUPPORTED)
}

// rtlGetLastNtStatus is ntdll's RtlGetLastNtStatus: the NTSTATUS of the last failed call of the calling thread, which
// tells a name whose delete is pending from one access refuses, both ERROR_ACCESS_DENIED.
var rtlGetLastNtStatus = windows.NewLazySystemDLL("ntdll.dll").NewProc("RtlGetLastNtStatus")

// deletePending runs call on one locked OS thread and reports whether it failed on a name whose delete is pending
// (STATUS_DELETE_PENDING). A remover — DeleteFileW, as os.Remove calls it — marks the file deleted through its own
// handle and the name goes when that handle closes, so between the two every open and every link of the name is
// refused with ERROR_ACCESS_DENIED; the window ends with the remover's call, or with its process.
func deletePending(call func() error) (bool, error) {
	if err := rtlGetLastNtStatus.Find(); err != nil {
		return false, call()
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	err := call()
	if err == nil || !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return false, err
	}
	status, _, _ := rtlGetLastNtStatus.Call()
	return windows.NTStatus(status) == windows.STATUS_DELETE_PENDING, err
}

// pendingDelete is a refusal of a name whose delete is pending, which is no refusal of access: it is fs.ErrNotExist where the name is read and
// fs.ErrExist where it is created, as the removal that already happened for its remover says.
type pendingDelete struct {
	err error
	is  error
}

func (e *pendingDelete) Error() string { return e.err.Error() + " (its delete is pending)" }

func (e *pendingDelete) Is(target error) bool { return target == e.is }

// linkFile links new to old, the hard link that makes the lock and an unlock's claim appear atomically. A link
// refused because a delete is pending finds no file when old is the name whose delete is pending, and a file at new
// otherwise.
func linkFile(old, new string) error {
	from, err := windows.UTF16PtrFromString(old)
	if err != nil {
		return &os.LinkError{Op: "link", Old: old, New: new, Err: err}
	}
	to, err := windows.UTF16PtrFromString(new)
	if err != nil {
		return &os.LinkError{Op: "link", Old: old, New: new, Err: err}
	}
	pending, err := deletePending(func() error { return windows.CreateHardLink(to, from, 0) })
	if err == nil {
		return nil
	}
	if pending {
		is := fs.ErrExist
		f, oerr := openShared(old, os.O_RDONLY, 0)
		switch {
		case oerr == nil:
			oerr = f.Close()
		case errors.Is(oerr, fs.ErrNotExist):
			is, oerr = fs.ErrNotExist, nil
		}
		if oerr != nil {
			return &os.LinkError{Op: "link", Old: old, New: new, Err: errors.Join(err, oerr)}
		}
		err = &pendingDelete{err: err, is: is}
	}
	return &os.LinkError{Op: "link", Old: old, New: new, Err: err}
}

// lstatFile reads the attributes of p without following a symbolic link at its name; a name whose delete is pending
// finds no file.
func lstatFile(p string) (os.FileInfo, error) {
	var fi os.FileInfo
	pending, err := deletePending(func() (err error) {
		fi, err = os.Lstat(p)
		return err
	})
	if pending {
		var pe *os.PathError
		if errors.As(err, &pe) {
			return nil, &os.PathError{Op: pe.Op, Path: pe.Path, Err: &pendingDelete{err: pe.Err, is: fs.ErrNotExist}}
		}
		return nil, &pendingDelete{err: err, is: fs.ErrNotExist}
	}
	return fi, err
}
