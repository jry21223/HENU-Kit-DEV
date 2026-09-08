package career

import "fmt"

func careerJobIsRelevant(job Job) bool {
	return job.MatchScore > 0 && len(job.MatchReasons) > 0
}

func careerScanSummary(attempted, succeeded, jobs, relevant int) string {
	failed := attempted - succeeded
	if failed > 0 {
		return fmt.Sprintf("已扫描 %d 个来源（%d 已响应，%d 暂不可用），发现 %d 个岗位，%d 个相关岗位", attempted, succeeded, failed, jobs, relevant)
	}
	return fmt.Sprintf("已扫描 %d 个来源，发现 %d 个岗位，%d 个相关岗位", attempted, jobs, relevant)
}
