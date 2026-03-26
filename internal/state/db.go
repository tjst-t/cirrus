package state

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	pool *pgxpool.Pool
}

func NewDB(ctx context.Context, dsn string) (*DB, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect to db: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &DB{pool: pool}, nil
}

func (db *DB) Close() {
	db.pool.Close()
}

func (db *DB) Migrate(ctx context.Context) error {
	_, err := db.pool.Exec(ctx, schema)
	return err
}

// Projects

func (db *DB) CreateProject(ctx context.Context, p *Project) error {
	return db.pool.QueryRow(ctx,
		`INSERT INTO projects (name, quota_vcpus, quota_ram_mb, quota_vms)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at, updated_at`,
		p.Name, p.QuotaVCPUs, p.QuotaRamMB, p.QuotaVMs,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (db *DB) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, name, quota_vcpus, quota_ram_mb, quota_vms, created_at, updated_at
		 FROM projects ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.QuotaVCPUs, &p.QuotaRamMB, &p.QuotaVMs, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, nil
}

// API Keys

func (db *DB) CreateAPIKey(ctx context.Context, projectID string, name string) (*APIKeyWithRaw, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	rawKey := "cirrus_" + hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	var ak APIKeyWithRaw
	ak.RawKey = rawKey
	err := db.pool.QueryRow(ctx,
		`INSERT INTO api_keys (project_id, name, key_hash)
		 VALUES ($1, $2, $3)
		 RETURNING id, project_id, name, created_at`,
		projectID, name, keyHash,
	).Scan(&ak.ID, &ak.ProjectID, &ak.Name, &ak.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &ak, nil
}

func (db *DB) AuthenticateKey(ctx context.Context, rawKey string) (string, error) {
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])
	var projectID string
	err := db.pool.QueryRow(ctx,
		`SELECT project_id FROM api_keys WHERE key_hash = $1`, keyHash,
	).Scan(&projectID)
	if err != nil {
		return "", fmt.Errorf("invalid api key")
	}
	return projectID, nil
}

// Workers

func (db *DB) UpsertWorker(ctx context.Context, w *Worker) error {
	return db.pool.QueryRow(ctx,
		`INSERT INTO workers (name, address, total_vcpus, total_ram_mb, total_disk_gb, status, last_heartbeat)
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 ON CONFLICT (name) DO UPDATE SET
		   address = EXCLUDED.address,
		   total_vcpus = EXCLUDED.total_vcpus,
		   total_ram_mb = EXCLUDED.total_ram_mb,
		   total_disk_gb = EXCLUDED.total_disk_gb,
		   status = EXCLUDED.status,
		   last_heartbeat = now(),
		   updated_at = now()
		 RETURNING id, created_at, updated_at, last_heartbeat`,
		w.Name, w.Address, w.TotalVCPUs, w.TotalRamMB, w.TotalDiskGB, w.Status,
	).Scan(&w.ID, &w.CreatedAt, &w.UpdatedAt, &w.LastHeartbeat)
}

func (db *DB) UpdateWorkerHeartbeat(ctx context.Context, name string) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE workers SET last_heartbeat = now(), status = 'online', updated_at = now()
		 WHERE name = $1`, name)
	return err
}

func (db *DB) ListWorkers(ctx context.Context) ([]Worker, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, name, address, total_vcpus, total_ram_mb, total_disk_gb, status, last_heartbeat, created_at, updated_at
		 FROM workers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workers []Worker
	for rows.Next() {
		var w Worker
		if err := rows.Scan(&w.ID, &w.Name, &w.Address, &w.TotalVCPUs, &w.TotalRamMB, &w.TotalDiskGB,
			&w.Status, &w.LastHeartbeat, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, nil
}

func (db *DB) GetWorker(ctx context.Context, id string) (*Worker, error) {
	var w Worker
	err := db.pool.QueryRow(ctx,
		`SELECT id, name, address, total_vcpus, total_ram_mb, total_disk_gb, status, last_heartbeat, created_at, updated_at
		 FROM workers WHERE id = $1`, id,
	).Scan(&w.ID, &w.Name, &w.Address, &w.TotalVCPUs, &w.TotalRamMB, &w.TotalDiskGB,
		&w.Status, &w.LastHeartbeat, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// Images

func (db *DB) CreateImage(ctx context.Context, img *Image) error {
	return db.pool.QueryRow(ctx,
		`INSERT INTO images (name, project_id, format, size_bytes, path, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at`,
		img.Name, img.ProjectID, img.Format, img.SizeBytes, img.Path, img.Status,
	).Scan(&img.ID, &img.CreatedAt)
}

func (db *DB) UpdateImageStatus(ctx context.Context, id string, status string, sizeBytes int64) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE images SET status = $2, size_bytes = $3 WHERE id = $1`,
		id, status, sizeBytes)
	return err
}

func (db *DB) ListImages(ctx context.Context, projectID string) ([]Image, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, name, project_id, format, size_bytes, path, status, created_at
		 FROM images WHERE project_id = $1 OR project_id IS NULL
		 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var images []Image
	for rows.Next() {
		var img Image
		if err := rows.Scan(&img.ID, &img.Name, &img.ProjectID, &img.Format, &img.SizeBytes,
			&img.Path, &img.Status, &img.CreatedAt); err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	return images, nil
}

func (db *DB) GetImage(ctx context.Context, id string) (*Image, error) {
	var img Image
	err := db.pool.QueryRow(ctx,
		`SELECT id, name, project_id, format, size_bytes, path, status, created_at
		 FROM images WHERE id = $1`, id,
	).Scan(&img.ID, &img.Name, &img.ProjectID, &img.Format, &img.SizeBytes,
		&img.Path, &img.Status, &img.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &img, nil
}

// Networks

func (db *DB) CreateNetwork(ctx context.Context, n *Network) error {
	return db.pool.QueryRow(ctx,
		`INSERT INTO networks (project_id, name, cidr, gateway, vni, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at`,
		n.ProjectID, n.Name, n.CIDR, n.Gateway, n.VNI, n.Status,
	).Scan(&n.ID, &n.CreatedAt)
}

func (db *DB) AllocateVNI(ctx context.Context) (int, error) {
	var vni int
	err := db.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(vni), 99) + 1 FROM networks`).Scan(&vni)
	if err != nil {
		return 0, err
	}
	return vni, nil
}

func (db *DB) ListNetworks(ctx context.Context, projectID string) ([]Network, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, project_id, name, cidr::text, host(gateway), vni, status, created_at
		 FROM networks WHERE project_id = $1 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nets []Network
	for rows.Next() {
		var n Network
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.Name, &n.CIDR, &n.Gateway, &n.VNI, &n.Status, &n.CreatedAt); err != nil {
			return nil, err
		}
		nets = append(nets, n)
	}
	return nets, nil
}

func (db *DB) GetNetwork(ctx context.Context, id string, projectID string) (*Network, error) {
	var n Network
	err := db.pool.QueryRow(ctx,
		`SELECT id, project_id, name, cidr::text, host(gateway), vni, status, created_at
		 FROM networks WHERE id = $1 AND project_id = $2`, id, projectID,
	).Scan(&n.ID, &n.ProjectID, &n.Name, &n.CIDR, &n.Gateway, &n.VNI, &n.Status, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (db *DB) DeleteNetwork(ctx context.Context, id string, projectID string) error {
	tag, err := db.pool.Exec(ctx,
		`DELETE FROM networks WHERE id = $1 AND project_id = $2`, id, projectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// VMs

func (db *DB) CreateVM(ctx context.Context, vm *VM) error {
	return db.pool.QueryRow(ctx,
		`INSERT INTO vms (project_id, name, image_id, vcpus, ram_mb, disk_gb, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at, updated_at`,
		vm.ProjectID, vm.Name, vm.ImageID, vm.VCPUs, vm.RamMB, vm.DiskGB, vm.Status,
	).Scan(&vm.ID, &vm.CreatedAt, &vm.UpdatedAt)
}

func (db *DB) UpdateVMStatus(ctx context.Context, id string, status string, errorMsg *string) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE vms SET status = $2, error_msg = $3, updated_at = now() WHERE id = $1`,
		id, status, errorMsg)
	return err
}

func (db *DB) UpdateVMWorker(ctx context.Context, id string, workerID string) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE vms SET worker_id = $2, status = 'building', updated_at = now() WHERE id = $1`,
		id, workerID)
	return err
}

func (db *DB) ListVMs(ctx context.Context, projectID string) ([]VM, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT v.id, v.project_id, v.name, v.worker_id, v.image_id, v.vcpus, v.ram_mb, v.disk_gb,
		        v.status, v.error_msg, v.storage_data, v.compute_data, v.created_at, v.updated_at
		 FROM vms v WHERE v.project_id = $1 AND v.status != 'deleted'
		 ORDER BY v.created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var vms []VM
	for rows.Next() {
		var vm VM
		if err := rows.Scan(&vm.ID, &vm.ProjectID, &vm.Name, &vm.WorkerID, &vm.ImageID,
			&vm.VCPUs, &vm.RamMB, &vm.DiskGB, &vm.Status, &vm.ErrorMsg,
			&vm.StorageData, &vm.ComputeData, &vm.CreatedAt, &vm.UpdatedAt); err != nil {
			return nil, err
		}
		vms = append(vms, vm)
	}
	return vms, nil
}

func (db *DB) GetVM(ctx context.Context, id string, projectID string) (*VM, error) {
	var vm VM
	err := db.pool.QueryRow(ctx,
		`SELECT id, project_id, name, worker_id, image_id, vcpus, ram_mb, disk_gb,
		        status, error_msg, storage_data, compute_data, created_at, updated_at
		 FROM vms WHERE id = $1 AND project_id = $2`, id, projectID,
	).Scan(&vm.ID, &vm.ProjectID, &vm.Name, &vm.WorkerID, &vm.ImageID,
		&vm.VCPUs, &vm.RamMB, &vm.DiskGB, &vm.Status, &vm.ErrorMsg,
		&vm.StorageData, &vm.ComputeData, &vm.CreatedAt, &vm.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &vm, nil
}

func (db *DB) GetVMByID(ctx context.Context, id string) (*VM, error) {
	var vm VM
	err := db.pool.QueryRow(ctx,
		`SELECT id, project_id, name, worker_id, image_id, vcpus, ram_mb, disk_gb,
		        status, error_msg, storage_data, compute_data, created_at, updated_at
		 FROM vms WHERE id = $1`, id,
	).Scan(&vm.ID, &vm.ProjectID, &vm.Name, &vm.WorkerID, &vm.ImageID,
		&vm.VCPUs, &vm.RamMB, &vm.DiskGB, &vm.Status, &vm.ErrorMsg,
		&vm.StorageData, &vm.ComputeData, &vm.CreatedAt, &vm.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &vm, nil
}

// Ports

func (db *DB) CreatePort(ctx context.Context, p *Port) error {
	return db.pool.QueryRow(ctx,
		`INSERT INTO ports (project_id, network_id, vm_id, mac_address, ip_address, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at`,
		p.ProjectID, p.NetworkID, p.VMID, p.MACAddress, p.IPAddress, p.Status,
	).Scan(&p.ID, &p.CreatedAt)
}

func (db *DB) ListPortsByVM(ctx context.Context, vmID string) ([]Port, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, project_id, network_id, vm_id, mac_address::text, host(ip_address), status, network_data, created_at
		 FROM ports WHERE vm_id = $1`, vmID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ports []Port
	for rows.Next() {
		var p Port
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.NetworkID, &p.VMID, &p.MACAddress,
			&p.IPAddress, &p.Status, &p.NetworkData, &p.CreatedAt); err != nil {
			return nil, err
		}
		ports = append(ports, p)
	}
	return ports, nil
}

func (db *DB) ListPortsByProject(ctx context.Context, projectID string) ([]Port, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, project_id, network_id, vm_id, mac_address::text, host(ip_address), status, network_data, created_at
		 FROM ports WHERE project_id = $1 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ports []Port
	for rows.Next() {
		var p Port
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.NetworkID, &p.VMID, &p.MACAddress,
			&p.IPAddress, &p.Status, &p.NetworkData, &p.CreatedAt); err != nil {
			return nil, err
		}
		ports = append(ports, p)
	}
	return ports, nil
}

func (db *DB) GetPort(ctx context.Context, id string, projectID string) (*Port, error) {
	var p Port
	err := db.pool.QueryRow(ctx,
		`SELECT id, project_id, network_id, vm_id, mac_address::text, host(ip_address), status, network_data, created_at
		 FROM ports WHERE id = $1 AND project_id = $2`, id, projectID,
	).Scan(&p.ID, &p.ProjectID, &p.NetworkID, &p.VMID, &p.MACAddress,
		&p.IPAddress, &p.Status, &p.NetworkData, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) UpdatePortStatus(ctx context.Context, id string, status string) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE ports SET status = $2 WHERE id = $1`, id, status)
	return err
}

func (db *DB) GetUsedIPs(ctx context.Context, networkID string) ([]string, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT host(ip_address) FROM ports WHERE network_id = $1`, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ips []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		ips = append(ips, ip)
	}
	return ips, nil
}

// Quota

type QuotaUsage struct {
	UsedVCPUs int
	UsedRamMB int
	UsedVMs   int
}

func (db *DB) GetQuotaUsage(ctx context.Context, projectID string) (*QuotaUsage, error) {
	var u QuotaUsage
	err := db.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(vcpus), 0), COALESCE(SUM(ram_mb), 0), COUNT(*)
		 FROM vms WHERE project_id = $1 AND status NOT IN ('deleted', 'error')`, projectID,
	).Scan(&u.UsedVCPUs, &u.UsedRamMB, &u.UsedVMs)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (db *DB) GetProject(ctx context.Context, id string) (*Project, error) {
	var p Project
	err := db.pool.QueryRow(ctx,
		`SELECT id, name, quota_vcpus, quota_ram_mb, quota_vms, created_at, updated_at
		 FROM projects WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.QuotaVCPUs, &p.QuotaRamMB, &p.QuotaVMs, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Scheduler helpers

type WorkerCapacity struct {
	Worker   Worker
	UsedVCPUs int
	UsedRamMB int
	UsedDiskGB int
}

func (db *DB) GetWorkerCapacities(ctx context.Context) ([]WorkerCapacity, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT w.id, w.name, w.address, w.total_vcpus, w.total_ram_mb, w.total_disk_gb,
		        w.status, w.last_heartbeat, w.created_at, w.updated_at,
		        COALESCE(SUM(v.vcpus), 0), COALESCE(SUM(v.ram_mb), 0), COALESCE(SUM(v.disk_gb), 0)
		 FROM workers w
		 LEFT JOIN vms v ON v.worker_id = w.id AND v.status NOT IN ('deleted', 'error')
		 WHERE w.status = 'online'
		 GROUP BY w.id
		 ORDER BY w.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var caps []WorkerCapacity
	for rows.Next() {
		var wc WorkerCapacity
		if err := rows.Scan(&wc.Worker.ID, &wc.Worker.Name, &wc.Worker.Address,
			&wc.Worker.TotalVCPUs, &wc.Worker.TotalRamMB, &wc.Worker.TotalDiskGB,
			&wc.Worker.Status, &wc.Worker.LastHeartbeat, &wc.Worker.CreatedAt, &wc.Worker.UpdatedAt,
			&wc.UsedVCPUs, &wc.UsedRamMB, &wc.UsedDiskGB); err != nil {
			return nil, err
		}
		caps = append(caps, wc)
	}
	return caps, nil
}

// GenerateMAC generates a locally administered MAC address.
func GenerateMAC() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	buf[0] = 0x02 // locally administered, unicast
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5]), nil
}
