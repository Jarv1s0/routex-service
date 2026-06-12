package cmd

import (
	"encoding/json"
	"fmt"
	routesys "routex-service/sys"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var tunFakeIPRanges string

var sysCmd = &cobra.Command{
	Use:   "sys",
	Short: "管理系统能力",
}

var tunCleanupCmd = &cobra.Command{
	Use:   "tun-cleanup",
	Short: "清理 TUN 虚拟网卡残留",
	Run: func(cmd *cobra.Command, args []string) {
		t := time.Now()
		result, err := routesys.CleanupTun(routesys.TunCleanupOptions{
			Device:       device,
			FakeIPRanges: splitCommaList(tunFakeIPRanges),
		})
		if err != nil {
			fmt.Println("清理 TUN 残留失败：", err)
			return
		}
		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Println("格式化 JSON 失败：", err)
			return
		}
		fmt.Println(string(resultJSON))
		fmt.Println("TUN 残留清理完成，耗时：", time.Since(t))
	},
}

func splitCommaList(value string) []string {
	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}
