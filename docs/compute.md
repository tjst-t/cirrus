# Compute設計

## libvirtバインディング

`digitalocean/go-libvirt` を使用（pure Go、cgo不要）。

単一バイナリ配布を重視するため、cgoが必要な公式バインディング (`libvirt.org/go/libvirt`) は使わない。

## interface

```go
// internal/compute/driver.go
package compute

type Driver interface {
    CreateVM(ctx context.Context, spec VMSpec) error
    DeleteVM(ctx context.Context, vmID string) error
    StopVM(ctx context.Context, vmID string) error
    StartVM(ctx context.Context, vmID string) error
    GetStatus(ctx context.Context, vmID string) (VMStatus, error)
    ListVMs(ctx context.Context) ([]VMStatus, error)
}

type VMSpec struct {
    ID        string
    Name      string
    VCPUs     int
    RamMB     int
    Disk      LibvirtDiskSpec  // storageが解決済み
    Ports     []PortSpec
    CloudInit []byte
}
```

## VM作成フロー

1. ベースイメージからVMディスク作成（storage.Backendが担当）
2. cloud-init ISO生成（ネットワーク設定、SSH鍵）
3. libvirt domain XML生成
4. domain define + start

## domain XML テンプレート

```xml
<domain type='kvm'>
  <name>cirrus-{{.VMID}}</name>
  <uuid>{{.VMID}}</uuid>
  <memory unit='MiB'>{{.RamMB}}</memory>
  <vcpu>{{.VCPUs}}</vcpu>
  <os>
    <type arch='x86_64'>hvm</type>
    <boot dev='hd'/>
  </os>
  <cpu mode='host-passthrough'/>
  <devices>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2' discard='unmap'/>
      <source file='{{.DiskPath}}'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <disk type='file' device='cdrom'>
      <source file='/var/lib/cirrus/cloud-init/{{.VMID}}.iso'/>
      <target dev='sda' bus='sata'/>
      <readonly/>
    </disk>
    {{range .Ports}}
    <interface type='bridge'>
      <mac address='{{.MAC}}'/>
      <source bridge='br-int'/>
      <virtualport type='openvswitch'>
        <parameters interfaceid='{{.ID}}'/>
      </virtualport>
      <model type='virtio'/>
    </interface>
    {{end}}
    <serial type='pty'/>
    <console type='pty'/>
  </devices>
</domain>
```

注: interfaceのtypeは `bridge` でOVSの `br-int` に直結。OVS側でportのUUIDをキーにVNIタグ付けをする。

## cloud-init

VMのIPアドレス設定はcloud-initのnetwork-configでstatic設定を渡す（DHCP不要でシンプル）。

```yaml
# network-config
version: 2
ethernets:
  ens2:
    addresses:
      - 10.100.0.5/24
    gateway4: 10.100.0.1
```

DHCPはPhase 2で検討（dnsmasqをテナントネットワークごとに起動する方式）。
