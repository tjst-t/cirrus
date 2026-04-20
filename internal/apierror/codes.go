package apierror

const (
	// スケジューラ系
	CodeNoHost               = "ERR_NO_HOST"
	CodeInsufficientResources = "ERR_INSUFFICIENT_RESOURCES"

	// クォータ系
	CodeQuotaVCPU          = "ERR_QUOTA_VCPU"
	CodeQuotaMemory        = "ERR_QUOTA_MEMORY"
	CodeQuotaVolumeGB      = "ERR_QUOTA_VOLUME_GB"
	CodeQuotaVMCount       = "ERR_QUOTA_VM_COUNT"
	CodeQuotaVolumeCount   = "ERR_QUOTA_VOLUME_COUNT"
	CodeQuotaSnapshotCount = "ERR_QUOTA_SNAPSHOT_COUNT"
	CodeQuotaNetworkCount  = "ERR_QUOTA_NETWORK_COUNT"
	CodeQuotaEgressCount   = "ERR_QUOTA_EGRESS_COUNT"
	CodeQuotaIngressCount  = "ERR_QUOTA_INGRESS_COUNT"
	CodeQuotaExceeded      = "ERR_QUOTA_EXCEEDED" // 汎用フォールバック

	// 認証認可系
	CodeUnauthorized = "ERR_UNAUTHORIZED"
	CodeForbidden    = "ERR_FORBIDDEN"

	// リソース系
	CodeNotFound     = "ERR_NOT_FOUND"
	CodeConflict     = "ERR_CONFLICT"       // 名前重複など作成時の競合
	CodeInvalidState = "ERR_INVALID_STATE"  // 現在の状態では操作不可（実行中VMの削除等）
	CodeBadRequest   = "ERR_BAD_REQUEST"

	// 内部エラー
	CodeInternal = "ERR_INTERNAL"
)

// ERR_INVALID_STATE の詳細理由（detail.reason フィールドに設定する）
const (
	ReasonVMRunning       = "vm_running"        // VMが実行中 → 先に停止が必要
	ReasonVMNotRunning    = "vm_not_running"     // VMが起動していない → 先に起動が必要
	ReasonVMNotStopped    = "vm_not_stopped"     // VMが停止していない → 先に停止が必要
	ReasonVMTransitional  = "vm_transitional"    // VMが遷移中（building/deleting等）
	ReasonHasDependents   = "has_dependents"     // 依存リソースが存在する → 先に削除が必要
	ReasonVolumeAttached  = "volume_attached"    // ボリュームがVMにアタッチ中 → 先にデタッチが必要
	ReasonIPInUse         = "ip_in_use"          // IPアドレスが使用中
	ReasonHostNotOperable = "host_not_operable"  // ホストが操作不可能な状態
)

// QuotaResourceToCode maps a quota resource name to its API error code.
func QuotaResourceToCode(resource string) string {
	switch resource {
	case "vcpu":
		return CodeQuotaVCPU
	case "memory_mb":
		return CodeQuotaMemory
	case "volume_gb":
		return CodeQuotaVolumeGB
	case "vm_count":
		return CodeQuotaVMCount
	case "volume_count":
		return CodeQuotaVolumeCount
	case "snapshot_count":
		return CodeQuotaSnapshotCount
	case "network_count":
		return CodeQuotaNetworkCount
	case "egress_count":
		return CodeQuotaEgressCount
	case "ingress_count":
		return CodeQuotaIngressCount
	default:
		return CodeQuotaExceeded
	}
}
