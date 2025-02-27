package main

import (
	"crypto/tls"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	h "net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/hub/executor"
	"github.com/Dreamacro/clash/listener/http" //编码转换
	"github.com/axgle/mahonia"
	"github.com/sjlleo/netflix-verify/verify"
	"github.com/xuri/excelize/v2"
)

var proxy constant.Proxy
var proxyUrl = "127.0.0.1:"
var exPath string

func getIP() string {

	proxy, _ := url.Parse("http://" + proxyUrl)
	client := h.Client{
		Timeout: 5 * time.Second,
		Transport: &h.Transport{
			// 设置代理
			Proxy: h.ProxyURL(proxy),
		},
	}
	resp, err := client.Get("http://myexternalip.com/raw")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	content, _ := ioutil.ReadAll(resp.Body)
	return string(content)
}

func relay(l, r net.Conn) {
	go io.Copy(l, r)
	io.Copy(r, l)
}

// 获取可用端口
func GetAvailablePort() (int, error) {
	address, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:0", "127.0.0.1"))
	if err != nil {
		return 0, err
	}

	listener, err := net.ListenTCP("tcp", address)
	if err != nil {
		return 0, err
	}

	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil

}

func downloadConfig() {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath = filepath.Dir(ex)
	fmt.Println(exPath)
	//输入订阅链接
	fmt.Println("请输入clash订阅链接(非clash订阅请进行订阅转换)")
	var urlConfig string
	_, err = fmt.Scanln(&urlConfig)
	if err != nil {
		panic(err)
	}
	tr := &h.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	//下载配置信息
	client := &h.Client{
		Timeout: 5 * time.Second,
		Transport: tr,
	}
	req, _ := h.NewRequest("GET", urlConfig, nil)
	// 设置 Clash User-Agent，方便面板尽可能识别为Clash并返回符合Clash的结果
	req.Header.Set("User-Agent", "Clash")

	res, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	// res, err := h.Get(urlConfig)
	if err != nil {
		fmt.Println("clash的订阅链接下载失败！")
		time.Sleep(10 * time.Second)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			panic(err)
		}
	}(res.Body)
	//创建配置文件
	f, err := os.OpenFile(exPath+"/config.yaml", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0777)
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			panic(err)
		}
	}(f)
	if err != nil {
		fmt.Println("clash的订阅链接下载失败！")
		time.Sleep(10 * time.Second)
		return
	}
	_, err = io.Copy(f, res.Body)
	if err != nil {
		panic(err)
	}
}

func main() {
	//downloadConfig()

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath = filepath.Dir(ex)
	//解析配置信息
	config, err := executor.ParseWithPath(exPath + "/config.yaml")
	if err != nil {
		panic(err)
		return
	}
	//获取端口
	port, _ := GetAvailablePort()
	proxyUrl += strconv.Itoa(port)
	//开启代理
	in := make(chan constant.ConnContext, 100)
	defer close(in)
	l, err := http.New(proxyUrl, in)
	if err != nil {
		panic(err)
	}
	defer l.Close()
	println("listen at:", l.Address())

	//设置编码
	enc := mahonia.NewDecoder("utf8")

	//监听代理
	go func() {
		for c := range in {
			conn := c
			metadata := conn.Metadata()
			go func() {
				remote, err := proxy.DialContext(context.Background(), metadata)
				if err != nil {
					conn.Conn().Close()
					return
				}
				relay(remote, conn.Conn())
			}()
		}
	}()

	//创建result.txt
	f, err := os.OpenFile(exPath+"/result.txt", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	defer f.Close()
	if err != nil {
		fmt.Println("新建result.txt失败：", err)
	}

	//创建excel
	excel := excelize.NewFile()
	excel.SetCellValue("Sheet1", "A1", "节点名")
	excel.SetCellValue("Sheet1", "B1", "ip地址")
	excel.SetCellValue("Sheet1", "C1", "netflix解锁")
	excel.SetCellValue("Sheet1", "D1", "说明")
	excel.SetCellValue("Sheet1", "E1", "disney解锁")
	excel.SetCellValue("Sheet1", "F1", "说明")

	index := 1
	nodes := config.Proxies

	for node, server := range nodes {

		var (
			unblock bool
			res     string
			unblock2 bool
			res2     string
		)
		if server.Type() != constant.Shadowsocks && server.Type() != constant.ShadowsocksR && server.Type() != constant.Snell && server.Type() != constant.Socks5 && server.Type() != constant.Http && server.Type() != constant.Vmess && server.Type() != constant.Trojan {
			continue
		}
		proxy = server
		//落地机IP
		ip := getIP()
		str := fmt.Sprintf("%d   节点名: %s ip地址:%s\n", index, node, ip)
		fmt.Print(str)

		//Netflix检测
		r := verify.NewVerify(verify.Config{
			Proxy: "http://" + proxyUrl,
		})
		
		
		switch vcode := r.Res[1].StatusCode; {
		case vcode == 2:
			unblock = true
			res = "【Netflix】IPv4完整解锁，可观看全部影片，区域：" + r.Res[1].CountryName
		case vcode == 1:
			unblock = true
			res = "【Netflix】IPv4部分解锁，可观看自制剧，区域：" + r.Res[1].CountryName
		case vcode == 0:
			unblock = false
			res = "【Netflix】IPv4无法正常使用"
		case vcode == -1:
			unblock = false
			res = "【Netflix】IPv4 IP所在的国家不提供服务"
		case vcode < -1:
			unblock = false
			res = "【Netflix】IPv4网络没有配置"
		default:
			unblock = false
			res = "【Netflix】IPv4解锁检测失败"
		}

		//fmt.Fprintln(f, enc.ConvertString(str+res))
		res += "\n"

		switch vcode := r.Res[2].StatusCode; {
		case vcode == 2:
			unblock = unblock || true
			res += "【Netflix】IPv6完整解锁，可观看全部影片，区域：" + r.Res[1].CountryName
		case vcode == 1:
			unblock = unblock || true
			res += "【Netflix】IPv6部分解锁，可观看自制剧，区域：" + r.Res[1].CountryName
		case vcode == 0:
			unblock = unblock || false
			res += "【Netflix】IPv6无法正常使用"
		case vcode == -1:
			unblock = unblock || false
			res += "【Netflix】IPv6 IP所在的国家不提供服务"
		case vcode < -1:
			unblock = unblock || false
			res += "【Netflix】IPv6网络没有配置"
		default:
			unblock = unblock || false
			res += "【Netflix】IPv6解锁检测失败"
		}

		fmt.Fprintln(f, enc.ConvertString(res))

		//disney
		QueryStatusv4 := QueryAreaAvailable("ipv4","http://" + proxyUrl)
		QueryStatusv6 := QueryAreaAvailable("ipv6","http://" + proxyUrl)

		VerifyStatus := VerifyAuthorized("http://" + proxyUrl)
		if VerifyStatus == -2 {
			fmt.Fprintln(f, "【disney】无法获取DisneyPlus权验接口信息，当前测试可能会不准确")
		}

		switch QueryStatusv4 {
		case "400":
			unblock2 = false
			break
		case "Unavailable":
			unblock2 = false
			res2 = "【disney】IPv4 IP所在的国家不提供服务"
			break
		case "-1":
			unblock2 = false
			res2 = "【disney】IPv4 IP所在的国家即将开通DisneyPlus"
			break
		default:
			//fmt.Fprintln(f, "[IPv4]")
			if VerifyStatus == -1 {
				unblock2 = false
				res2 = "【disney】IPv4无法正常使用"
			} else {
				unblock2 = true
				res2 = "【disney】IPv4完整解锁 区域：" + QueryStatusv4 + "区"
			}
		}
		
		res2 += "\n"

		switch QueryStatusv6 {
		case "400":
			unblock2 = unblock2 || false
			break
		case "Unavailable":
			// if NextLineSignal == true {
			// 	fmt.Fprintln(f, "\n")
			// }
			unblock2 = unblock2 || false
			res2 += "【disney】IPv6 IP所在的国家不提供服务"
			break
		case "-1":
			// if NextLineSignal == true {
			// 	fmt.Fprintln(f, "\n")
			// }
			unblock2 = unblock2 ||  false
			res2 += "【disney】IPv6IP所在的国家即将开通DisneyPlus"
			break
		default:
			// if NextLineSignal == true {
			// 	fmt.Fprintln(f, "\n")
			// }
			//fmt.Fprintln(f, "[IPv6]")
			if VerifyStatus == -1 {
				unblock2 = unblock2 || false
				res2 += "【disney】IPv6 无法正常使用"
			} else {
				unblock2 = unblock2 || true
				res2 += "【disney】IPv6完整解锁 区域：" + QueryStatusv6 + "区"
			}
		}
		
		fmt.Fprintln(f, enc.ConvertString(res2))

		//write to excel
		excel.SetCellValue("Sheet1", "A"+strconv.Itoa(index+1), node)
		excel.SetCellValue("Sheet1", "B"+strconv.Itoa(index+1), ip)
		excel.SetCellValue("Sheet1", "C"+strconv.Itoa(index+1), unblock)
		excel.SetCellValue("Sheet1", "D"+strconv.Itoa(index+1), res)
		excel.SetCellValue("Sheet1", "E"+strconv.Itoa(index+1), unblock2)
		excel.SetCellValue("Sheet1", "F"+strconv.Itoa(index+1), res2)

		index++

		if err := excel.SaveAs(exPath + "/result.xlsx"); err != nil {
			fmt.Println(err)
		}
	}

	if err := excel.SaveAs(exPath + "/result.xlsx"); err != nil {
		fmt.Println(err)
	}
}
