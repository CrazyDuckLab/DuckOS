//go:build linux

// Package restore implements confined file restoration. It never executes commands.
package restore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"strings"

	"golang.org/x/sys/unix"
)

const maxFile = 1 << 20

func validPath(name string) bool {
	return name != "." && name != "" && !strings.Contains(name, "\\") && !strings.ContainsRune(name, 0) && !strings.HasPrefix(name, "/") && path.Clean(name) == name && name != ".." && !strings.HasPrefix(name, "../")
}

type root struct {
	f     *os.File
	limit int64
}

func openRoot(name string) (*root, error) {
	// Configured roots may be mount points, but no symlink is accepted while
	// opening them. Every subsequent resolution also refuses mount crossings.
	fd, err := unix.Openat2(unix.AT_FDCWD, name, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_NO_SYMLINKS})
	if errors.Is(err, unix.ENOSYS) {
		fd, err = walkNoFollow(unix.AT_FDCWD, name, unix.O_RDONLY|unix.O_DIRECTORY, false)
	}
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil || st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
		f.Close()
		return nil, errors.New("unsafe managed root")
	}
	return &root{f: f, limit: maxFile}, nil
}

func (r *root) open(name string, flags int) (*os.File, error) {
	if !validPath(name) {
		return nil, errors.New("invalid relative target")
	}
	fd, err := unix.Openat2(int(r.f.Fd()), name, &unix.OpenHow{Flags: uint64(flags | unix.O_CLOEXEC | unix.O_NOFOLLOW), Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
	if errors.Is(err, unix.ENOSYS) {
		fd, err = walkNoFollow(int(r.f.Fd()), name, flags, true)
	}
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

// systemd RestrictSUIDSGID may reject openat2 with ENOSYS because its flags
// live in an indirect structure. Keep the restriction: walk pinned descriptors
// with O_NOFOLLOW instead, and compare statx mount IDs at every component.
// Unlike st_dev, mount IDs also detect same-filesystem bind mounts.
func walkNoFollow(base int, name string, flags int, confined bool) (int, error) {
	var fd int
	var err error
	if confined {
		if !validPath(name) {
			return -1, unix.EINVAL
		}
		fd, err = unix.FcntlInt(uintptr(base), unix.F_DUPFD_CLOEXEC, 0)
	} else {
		if !strings.HasPrefix(name, "/") || !validPath(strings.TrimPrefix(name, "/")) {
			return -1, unix.EINVAL
		}
		name = strings.TrimPrefix(name, "/")
		fd, err = unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	}
	if err != nil {
		return -1, err
	}
	mountID := uint64(0)
	if confined {
		mountID, err = descriptorMountID(fd)
		if err != nil {
			unix.Close(fd)
			return -1, err
		}
	}
	parts := strings.Split(name, "/")
	for i, part := range parts {
		openFlags := unix.O_RDONLY | unix.O_DIRECTORY
		if i == len(parts)-1 {
			openFlags = flags
		}
		next, e := unix.Openat(fd, part, openFlags|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
		if confined {
			observed, e := descriptorMountID(fd)
			if e != nil || observed != mountID {
				unix.Close(fd)
				return -1, errors.Join(unix.EXDEV, e)
			}
			if i < len(parts)-1 {
				var st unix.Stat_t
				if e := unix.Fstat(fd, &st); e != nil || st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
					unix.Close(fd)
					return -1, unix.EACCES
				}
			}
		}
	}
	return fd, nil
}

func descriptorMountID(fd int) (uint64, error) {
	var st unix.Statx_t
	if err := unix.Statx(fd, "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &st); err != nil {
		return 0, err
	}
	if st.Mask&unix.STATX_MNT_ID == 0 {
		return 0, errors.New("mount identity unavailable")
	}
	return st.Mnt_id, nil
}

type image struct {
	Exists bool   `json:"exists"`
	Data   []byte `json:"data,omitempty"`
	Mode   uint32 `json:"mode,omitempty"`
	GID    int    `json:"gid"`
}

func (r *root) read(name string) (image, error) {
	f, err := r.open(name, unix.O_RDONLY|unix.O_NONBLOCK)
	if errors.Is(err, os.ErrNotExist) {
		return image{}, nil
	}
	if err != nil {
		return image{}, err
	}
	defer f.Close()
	var st unix.Stat_t
	if err = unix.Fstat(int(f.Fd()), &st); err != nil {
		return image{}, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Size > r.limit || st.Uid != uint32(os.Geteuid()) {
		return image{}, errors.New("unsafe managed file")
	}
	if st.Mode&(unix.S_ISUID|unix.S_ISGID|unix.S_ISVTX) != 0 {
		return image{}, errors.New("privilege-bearing file modes are not restorable")
	}
	if count, e := unix.Flistxattr(int(f.Fd()), nil); e != nil || count != 0 {
		return image{}, errors.New("extended file metadata requires an explicit restore provider")
	}
	b, err := io.ReadAll(io.LimitReader(f, r.limit+1))
	if err != nil || int64(len(b)) > r.limit {
		return image{}, errors.New("managed file exceeds bound")
	}
	return image{true, b, uint32(st.Mode & 0777), int(st.Gid)}, nil
}

func (r *root) parent(name string) (*os.File, error) {
	if !validPath(name) {
		return nil, errors.New("invalid relative target")
	}
	// Require pre-provisioned directories. Never create directory trees from a
	// request. Holding the descriptor prevents a symlink swap redirecting writes.
	parent := path.Dir(name)
	var dir *os.File
	var err error
	if parent == "." {
		var fd int
		fd, err = unix.Dup(int(r.f.Fd()))
		if err == nil {
			dir = os.NewFile(uintptr(fd), "parent")
		}
	} else {
		// Check every ancestor's ownership before pinning the leaf parent.
		components := strings.Split(parent, "/")
		for i := range components {
			p, e := r.open(strings.Join(components[:i+1], "/"), unix.O_RDONLY|unix.O_DIRECTORY)
			if e != nil {
				return nil, e
			}
			var st unix.Stat_t
			e = unix.Fstat(int(p.Fd()), &st)
			p.Close()
			if e != nil || st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
				return nil, errors.New("unsafe target ancestor")
			}
		}
		dir, err = r.open(parent, unix.O_RDONLY|unix.O_DIRECTORY)
	}
	if err != nil {
		return nil, err
	}
	// Directory entries below a root are root-owned and non-writable by others;
	// verify the parent so an unprivileged rename cannot move it outside the root.
	var st unix.Stat_t
	if err = unix.Fstat(int(dir.Fd()), &st); err != nil {
		dir.Close()
		return nil, err
	}
	if st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
		dir.Close()
		return nil, errors.New("unsafe target parent")
	}
	return dir, nil
}

func (r *root) write(name string, v image) error {
	dir, err := r.parent(name)
	if err != nil {
		return err
	}
	defer dir.Close()
	if _, err = r.read(name); err != nil {
		return err
	}
	base := path.Base(name)
	if !v.Exists {
		err = unix.Unlinkat(int(dir.Fd()), base, 0)
		if err != nil && !errors.Is(err, unix.ENOENT) {
			return err
		}
		return dir.Sync()
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return err
	}
	temp := ".restore-" + hex.EncodeToString(random[:])
	fd, err := unix.Openat(int(dir.Fd()), temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), temp)
	defer unix.Unlinkat(int(dir.Fd()), temp, 0)
	if _, err = f.Write(v.Data); err == nil {
		err = f.Chown(-1, v.GID)
	}
	if err == nil {
		err = f.Chmod(os.FileMode(v.Mode & 0777))
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err = unix.Renameat(int(dir.Fd()), temp, int(dir.Fd()), base); err != nil {
		return err
	}
	return dir.Sync()
}
