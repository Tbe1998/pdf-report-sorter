//go:build !windows

package main

// 非 Windows 平台的命令行入口，用于在 Linux 上验证分拣逻辑。
import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	src := flag.String("src", "", "监听文件夹")
	rules := flag.String("rules", "", "规则文件路径")
	dst := flag.String("dst", "", "默认目标文件夹（可空）")
	filter := flag.String("filter", "", "只处理文件名包含该词的 PDF（可空）")
	settle := flag.Int("settle", 2, "稳定判定秒数")
	moveOld := flag.Bool("move-existing", false, "搬移启动时已存在的 PDF")
	watch := flag.Int("watch", 0, "运行多少秒后退出（0 表示一直运行）")
	byMonth := flag.Bool("bymonth", true, "按月份归档（目标文件夹\\26.9）")
	checkURL := flag.String("checkupdate", "", "检查更新并下载解包（测试用），传 latest.json 的网址")
	flag.Parse()

	if *checkURL != "" {
		man, err := fetchManifest(*checkURL)
		if err != nil {
			fmt.Println("检查更新失败：" + err.Error())
			os.Exit(1)
		}
		fmt.Printf("远端版本 %s，本地版本 %s，需要更新：%v\n", man.Version, appVersion, compareVersion(man.Version, appVersion) > 0)
		fmt.Printf("更新包 %s（%d 字节，sha256=%s）\n", man.Package, man.PackageSize, man.PackageSHA256)
		pkg, err := downloadPackage(man)
		if err != nil {
			fmt.Println("下载失败：" + err.Error())
			os.Exit(1)
		}
		dir := filepath.Join(os.TempDir(), "pdfmover-update-verify")
		os.RemoveAll(dir)
		if err := extractZip(pkg, dir); err != nil {
			fmt.Println("解压失败：" + err.Error())
			os.Exit(1)
		}
		exe, err := findExeIn(dir, man.ExeName)
		if err != nil {
			fmt.Println("找不到 exe：" + err.Error())
			os.Exit(1)
		}
		st, _ := os.Stat(exe)
		fmt.Printf("解压后主程序：%s（%d 字节）\n", filepath.Base(exe), st.Size())
		entries, _ := os.ReadDir(dir)
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		fmt.Printf("包内文件：%v\n", names)
		os.Exit(0)
	}

	logf := func(s string) { fmt.Printf("%s  %s\n", time.Now().Format("15:04:05"), s) }

	r := NewRouter(*rules)
	r.SetDefault(*dst, *filter)
	// 规则文件不存在时，用 -rule 参数快速建一个（方便测试）
	if _, err := os.Stat(*rules); err != nil {
		_ = os.MkdirAll(filepath.Dir(*rules), 0o755)
		_ = os.WriteFile(*rules, []byte(RuleTemplate), 0o644)
	}
	n, err := r.Load()
	if err != nil {
		fmt.Println("读取规则失败：" + err.Error())
		os.Exit(1)
	}
	logf(fmt.Sprintf("已加载 %d 条规则", n))

	m := NewMover()
	m.SetByMonth(*byMonth)
	if err := m.Start(*src, *settle, *moveOld, r, logf); err != nil {
		fmt.Println("启动失败：" + err.Error())
		os.Exit(1)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	if *watch > 0 {
		select {
		case <-time.After(time.Duration(*watch) * time.Second):
		case <-ch:
		}
		m.Stop()
	} else {
		<-ch
		m.Stop()
	}
	mv, fl, sk := m.Stats()
	fmt.Printf("完成：成功 %d 个，失败 %d 个，未匹配 %d 个\n", mv, fl, sk)
}
