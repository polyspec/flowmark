//go:build windows

package journal

import (
	"errors"
	"fmt"
	"os"

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
	h, err := windows.CreateFile(name, access, shareAll, nil, disposition, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: p, Err: err}
	}
	return os.NewFile(uintptr(h), p), nil
}

// fileIdentity is not used on Windows: the wait watches the folder, not the lock file.
func fileIdentity(*os.File) (fileID, error) { return fileID{}, nil }

// moveDurable moves src to dst with MoveFileEx and MOVEFILE_WRITE_THROUGH, which returns once the move is on disk;
// Windows has no folder flush, so this is how a name is made durable there.
func moveDurable(src, dst string) error {
	return moveFileEx(src, dst, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

// moveFolderDurable moves a folder to a name that does not exist; MOVEFILE_REPLACE_EXISTING cannot be used for a
// folder.
func moveFolderDurable(src, dst string) error {
	return moveFileEx(src, dst, windows.MOVEFILE_WRITE_THROUGH)
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
