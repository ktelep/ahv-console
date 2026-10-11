# Linux arm64 qualification on an Apple Silicon Mac

An ARM64 Ubuntu VM on an Apple Silicon Mac can provide native Linux arm64
evidence when the hypervisor uses ARM virtualization. Choose **Virtualize** in
UTM, or an equivalent ARM virtualization mode in another hypervisor. Do not
use x86_64 emulation for this qualification.

## Guest setup

- Install a generic ARM64/aarch64 Ubuntu image supported by your hypervisor.
  Avoid Raspberry Pi-specific images for a generic virtual machine.
- If using Ubuntu Server 24.04 ARM64, install the desktop after OS setup:

  ```sh
  sudo apt update
  sudo apt install ubuntu-desktop
  sudo systemctl set-default graphical.target
  sudo reboot
  ```

- Allocate approximately 4 virtual CPUs, 8 GiB RAM, and 64 GiB disk.
- Confirm the guest can reach your PC endpoint on its configured HTTPS port.
  NAT or bridged
  networking may work; VPN routing depends on your Mac/hypervisor configuration.
- Use a current browser in the guest's graphical desktop session.

## Copy and run the standalone binary

Run these commands **inside the Ubuntu VM**, replacing the SSH placeholders
with the development-machine details:

```sh
scp <ssh-user>@<build-host>:/path/to/ahv-console-linux-arm64 ~/Downloads/ahv-console
chmod +x ~/Downloads/ahv-console
uname -m
cat /etc/os-release
~/Downloads/ahv-console --version
~/Downloads/ahv-console --help
~/Downloads/ahv-console --licenses
~/Downloads/ahv-console 'Exact VM Name' --pc-host pc.example.com --username console-user
```

`uname -m` must report `aarch64` (or an equivalent ARM64 identifier). The PC
password is entered at the hidden prompt. Use `--ca-cert` for an enterprise CA,
or explicitly opt into `--insecure` for an appropriate self-signed test endpoint.
No application runtime or asset
files should be installed alongside the executable.

## Report results

Record the guest OS/version, architecture, hypervisor and virtualization mode,
browser/version, and binary SHA-256 (`sha256sum ~/Downloads/ahv-console`). Check:

1. Offline help/version/license commands start successfully.
2. The default browser opens automatically in the Ubuntu desktop.
3. VM screen updates, keyboard, and pointer input work.
4. Browser refresh reconnects while the command remains running.
5. Ctrl+C in the guest terminal stops the command and disconnects the viewer.
6. No separate application assets, VNC proxy, Go, or Node.js installation is
   required to run the copied executable.

Successful native startup and desktop interaction qualify Linux arm64. They do
not qualify the macOS amd64 executable, which remains a separate target check.
