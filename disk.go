package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) diskDiagnosis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	resp := DiskDiagnosisResponse{CheckedAt: time.Now().Format(time.RFC3339)}
	dfCtx, dfCancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer dfCancel()
	if output, err := exec.CommandContext(dfCtx, "df", "-h", "--output=target,size,used,pcent", "/").CombinedOutput(); err == nil {
		for i, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if i == 0 || strings.TrimSpace(line) == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				percent := 0
				fmt.Sscanf(strings.TrimSuffix(fields[3], "%"), "%d", &percent)
				resp.Filesystems = append(resp.Filesystems, DiskFilesystemInfo{Mount: fields[0], Total: fields[1], Used: fields[2], Percent: percent})
			}
		}
	}
	dockerCtx, dockerCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer dockerCancel()
	if output, err := exec.CommandContext(dockerCtx, "docker", "system", "df", "--format", "{{.Type}}\t{{.TotalCount}}\t{{.Size}}\t{{.Reclaimable}}").CombinedOutput(); err == nil {
		resp.DockerOK = true
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			switch fields[0] {
			case "Images":
				resp.Docker.ImagesTotal, _ = strconv.Atoi(fields[1])
				resp.Docker.ImagesSize = fields[2]
				resp.Docker.ImagesReclaimable = fields[3]
			case "BuildCache", "Build Cache":
				resp.Docker.BuildCacheSize = fields[2]
				resp.Docker.BuildReclaimable = fields[3]
			case "LocalVolumes", "Local Volumes":
				resp.Docker.VolumesSize = fields[2]
				resp.Docker.VolumesReclaimable = fields[3]
			}
		}
	}
	duCtx, duCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer duCancel()
	if output, err := exec.CommandContext(duCtx, "du", "-sh", "/var/lib/docker", "/opt", "/tmp", "/var/log", "/root").CombinedOutput(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				resp.TopDirs = append(resp.TopDirs, DiskUsageItem{Size: fields[0], Path: fields[1], Bytes: uint64(parseHumanBytes(fields[0]))})
			}
		}
	}

	// Enhanced: collect top 20 Docker images sorted by size
	imgCtx, imgCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer imgCancel()
	if output, err := exec.CommandContext(imgCtx, "docker", "images", "--format", "{{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedAt}}\t{{.ID}}").CombinedOutput(); err == nil {
		var allImages []DockerImageInfo
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fields := strings.SplitN(line, "\t", 5)
			if len(fields) < 5 {
				continue
			}
			repo := strings.TrimSpace(fields[0])
			tag := strings.TrimSpace(fields[1])
			info := DockerImageInfo{
				Repository: repo,
				Tag:        tag,
				Size:       strings.TrimSpace(fields[2]),
				CreatedAt:  strings.TrimSpace(fields[3]),
				ID:         strings.TrimSpace(fields[4]),
				Dangling:   repo == "<none>" && tag == "<none>",
			}
			allImages = append(allImages, info)
		}
		sort.Slice(allImages, func(i, j int) bool {
			return parseHumanBytes(allImages[i].Size) > parseHumanBytes(allImages[j].Size)
		})
		if len(allImages) > 20 {
			allImages = allImages[:20]
		}
		resp.Images = allImages
	}

	// Enhanced: collect Docker volumes and identify orphans
	volCtx, volCancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer volCancel()
	if output, err := exec.CommandContext(volCtx, "docker", "volume", "ls", "--format", "{{.Name}}\t{{.Driver}}\t{{.Mountpoint}}").CombinedOutput(); err == nil {
		usedMounts := map[string]bool{}
		if inspectOut, inspectErr := exec.CommandContext(volCtx, "docker", "ps", "--format", "{{.Mounts}}").CombinedOutput(); inspectErr == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(inspectOut)), "\n") {
				for _, part := range strings.Split(line, ",") {
					part = strings.TrimSpace(part)
					if strings.HasPrefix(part, "/") {
						usedMounts[part] = true
					}
				}
			}
		}
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fields := strings.SplitN(line, "\t", 3)
			if len(fields) < 3 {
				continue
			}
			name := strings.TrimSpace(fields[0])
			mountpoint := strings.TrimSpace(fields[2])
			info := DockerVolumeInfo{
				Name:       name,
				Driver:     strings.TrimSpace(fields[1]),
				Mountpoint: mountpoint,
				Orphan:     mountpoint != "" && !usedMounts[mountpoint],
			}
			resp.Volumes = append(resp.Volumes, info)
		}
	}

	// Enhanced: collect container log file sizes (top 10)
	logCtx, logCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer logCancel()
	if output, err := exec.CommandContext(logCtx, "du", "-sh", "/var/lib/docker/containers/*/*.log").CombinedOutput(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				resp.LogFiles = append(resp.LogFiles, DiskUsageItem{Size: fields[0], Path: fields[1], Bytes: uint64(parseHumanBytes(fields[0]))})
			}
		}
		sort.Slice(resp.LogFiles, func(i, j int) bool {
			return resp.LogFiles[i].Bytes > resp.LogFiles[j].Bytes
		})
		if len(resp.LogFiles) > 10 {
			resp.LogFiles = resp.LogFiles[:10]
		}
	}

	// Build waste breakdown summary
	if resp.DockerOK {
		total := parseHumanBytes(resp.Docker.BuildReclaimable) + parseHumanBytes(resp.Docker.ImagesReclaimable) + parseHumanBytes(resp.Docker.VolumesReclaimable)
		var logSizes []string
		for _, lf := range resp.LogFiles {
			total += float64(lf.Bytes)
			logSizes = append(logSizes, lf.Size)
		}
		resp.WasteBreakdown = &DiskWasteBreakdown{
			BuildCache:       resp.Docker.BuildReclaimable,
			DanglingImages:   resp.Docker.ImagesReclaimable,
			OrphanVolumes:    resp.Docker.VolumesReclaimable,
			ContainerLogs:    combineReclaimable(logSizes...),
			TotalReclaimable: formatBytes(total),
		}
	}

	writeJSON(w, resp)
}

func (a *App) diskCleanupPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	resp := DiskCleanupPreviewResponse{Recommendation: "build-cache"}
	dockerCtx, dockerCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer dockerCancel()
	if output, err := exec.CommandContext(dockerCtx, "docker", "system", "df", "--format", "{{.Type}}\t{{.TotalCount}}\t{{.Size}}\t{{.Reclaimable}}").CombinedOutput(); err == nil {
		resp.DockerOK = true
		var buildReclaimable, imagesReclaimable, volumesReclaimable string
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			switch fields[0] {
			case "Images":
				imagesReclaimable = fields[3]
			case "BuildCache", "Build Cache":
				buildReclaimable = fields[3]
			case "LocalVolumes", "Local Volumes":
				volumesReclaimable = fields[3]
			}
		}
		resp.Levels = []DiskCleanupLevel{
			{Level: "build-cache", Description: "构建缓存", Reclaimable: buildReclaimable, Command: "docker builder prune --all --force", Risk: "低，仅清理构建缓存"},
			{Level: "dangling-images", Description: "悬空镜像", Reclaimable: imagesReclaimable, Command: "docker image prune -f", Risk: "低，仅删除 <none> 标签镜像"},
			{Level: "standard", Description: "标准清理（悬空镜像+停止容器+build cache）", Reclaimable: combineReclaimable(buildReclaimable, imagesReclaimable), Command: "docker system prune -f", Risk: "中"},
			{Level: "orphan-volumes", Description: "孤儿卷", Reclaimable: volumesReclaimable, Command: "docker volume prune -f", Risk: "中，删除未被任何容器使用的卷"},
			{Level: "deep", Description: "深度清理（未使用镜像+卷+build cache）", Reclaimable: combineReclaimable(buildReclaimable, imagesReclaimable, volumesReclaimable), Command: "docker system prune -af --volumes", Risk: "高，会删除所有未使用的镜像和卷"},
		}
	}
	writeJSON(w, resp)
}

func (a *App) diskCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req DiskCleanupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Confirm != "CLEAN" {
		writeError(w, http.StatusBadRequest, "请输入确认词 CLEAN")
		return
	}
	level := strings.ToLower(strings.TrimSpace(req.Level))

	type cleanupStep struct {
		Category string
		Args     []string
		Timeout  time.Duration
	}
	var steps []cleanupStep
	switch level {
	case "build-cache":
		steps = []cleanupStep{{Category: "build_cache", Args: []string{"docker", "builder", "prune", "--all", "--force"}, Timeout: 30 * time.Second}}
	case "dangling-images":
		steps = []cleanupStep{{Category: "dangling_images", Args: []string{"docker", "image", "prune", "-f"}, Timeout: 30 * time.Second}}
	case "orphan-volumes":
		steps = []cleanupStep{{Category: "orphan_volumes", Args: []string{"docker", "volume", "prune", "-f"}, Timeout: 30 * time.Second}}
	case "standard":
		steps = []cleanupStep{{Category: "standard", Args: []string{"docker", "system", "prune", "-f"}, Timeout: 60 * time.Second}}
	case "deep":
		steps = []cleanupStep{{Category: "deep", Args: []string{"docker", "system", "prune", "-af", "--volumes"}, Timeout: 120 * time.Second}}
	default:
		writeError(w, http.StatusBadRequest, "无效的清理级别，可选：build-cache / dangling-images / orphan-volumes / standard / deep")
		return
	}

	var breakdown []DiskCleanupBreakdownItem
	totalReclaimed := float64(0)
	allDetails := []string{}
	var overallErr error

	for _, step := range steps {
		stepCtx, stepCancel := context.WithTimeout(r.Context(), step.Timeout)
		output, err := exec.CommandContext(stepCtx, step.Args[0], step.Args[1:]...).CombinedOutput()
		stepCancel()
		item := DiskCleanupBreakdownItem{
			Category: step.Category,
			Command:  strings.Join(step.Args, " "),
			Success:  err == nil,
		}
		if err != nil {
			item.Error = err.Error()
			if overallErr == nil {
				overallErr = err
			}
		}
		reclaimed := extractReclaimedSize(string(output))
		if reclaimed != "" {
			item.Reclaimed = reclaimed
			totalReclaimed += parseHumanBytes(reclaimed)
		}
		detail := strings.TrimSpace(string(output))
		if len(detail) > 300 {
			detail = detail[:300] + "..."
		}
		if detail != "" {
			allDetails = append(allDetails, detail)
		}
		breakdown = append(breakdown, item)
	}

	details := strings.Join(allDetails, "\n")
	if len(details) > 500 {
		details = details[:500] + "..."
	}
	reclaimedStr := formatBytes(totalReclaimed)

	_ = a.writeAudit(AuditRecord{
		Time: time.Now().Format(time.RFC3339), UserID: user.ID, Username: user.Username, RemoteIP: remoteIP(r),
		TaskID: "disk-cleanup-" + level, TaskTitle: "磁盘清理",
		Variables: map[string]string{"level": level, "reclaimed": reclaimedStr},
		Status:    ternaryText(overallErr == nil, "ok", "error"),
		Error:     ternaryText(overallErr != nil, overallErr.Error(), ""),
	})
	if overallErr != nil {
		writeError(w, http.StatusBadGateway, "清理失败："+overallErr.Error())
		return
	}
	writeJSON(w, DiskCleanupResponse{OK: true, Level: level, Reclaimed: reclaimedStr, Details: details, Breakdown: breakdown})
}

func combineReclaimable(values ...string) string {
	total := float64(0)
	for _, v := range values {
		total += parseHumanBytes(v)
	}
	return formatBytes(total)
}

func formatBytes(total float64) string {
	if total <= 0 {
		return "0B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	size := total
	for size >= 1000 && unitIndex < len(units)-1 {
		size /= 1000
		unitIndex++
	}
	return fmt.Sprintf("%.1f%s", size, units[unitIndex])
}

func extractReclaimedSize(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Reclaimed") || strings.Contains(line, "reclaimed") || strings.Contains(line, "Total reclaimed") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

