
package main

import (
	"github.com/hoisie/mustache"
	"github.com/axgle/mahonia"

	"bytes"
	"bufio"
	"fmt"
	"sync"
	"log"
	"strings"
	"os"
	"regexp"
	"errors"
	"io"
	"time"
	"io/ioutil"
	"encoding/json"
	"path/filepath"
	"net/http"
	"net"
)

func (m *vfile) log(format string, v ...interface{}) {
	str := fmt.Sprintf(format, v...)
	log.Printf("vfile %s: %s", m.sha, str)
}

func (m *vfile) parseM3u8() (ts []tsinfo) {
	lines := strings.Split(m.m3u8body, "\n")
	var dur float32

	for _, l := range lines {
		if strings.HasPrefix(l, "#EXTINF:") {
			fmt.Sscanf(l, "#EXTINF:%f", &dur)
		}
		if strings.HasPrefix(l, "http") {
			ts = append(ts, tsinfo{
				url:strings.TrimRight(l, "\r"),
				Dur:dur,
			})
			dur = 0
		}
	}
	return
}

func gbk2utf(in string) (out string) {
	b := bytes.NewBuffer([]byte(in))
	decoder := mahonia.NewDecoder("gbk")
	r := bufio.NewReader(decoder.NewReader(b))
	line, _, err := r.ReadLine()
	if err != nil {
		log.Printf("gtk2utf: err %v", err)
	}
	out = string(line)
	return
}

func (m *vfile) parseSohu() (err error) {

	m.Type = "sohu"

	var body string
	var re *regexp.Regexp
	var ma []string

	body, err = curl(m.Url)

	if err != nil {
		return errors.New(fmt.Sprintf("fetch index failed: %v", err))
	}

	if false {
		f, _ := os.Create("/tmp/sohu")
		data, _ := curldata(m.Url)
		f.Write(data)
		f.Close()
	}

	re, err = regexp.Compile(`<title>([^<]+)</title>`)
	ma = re.FindStringSubmatch(body)
	if len(ma) >= 2 {
		m.Desc = gbk2utf(ma[1])
		log.Printf("sohu: %s", m.Desc)
	}

	re, err = regexp.Compile(`var vid="([^"]+)"`)
	ma = re.FindStringSubmatch(body)

	if len(ma) != 2 {
		return errors.New(fmt.Sprintf("sohu: cannot find vid: ma = %v", ma))
	}

	vid := ma[1]

	m3u8url := "http://hot.vrs.sohu.com/ipad"+vid+".m3u8"
	body, err = curl(m3u8url)
	if m.err != nil {
		return errors.New(fmt.Sprintf("fetch m3u8 failed: %v", err))
	}
	m.m3u8body = body

	if false {
		f, _ := os.Create("/tmp/m3u8")
		fmt.Fprintf(f, "%v", body)
		f.Close()
	}

	return
}

func (m *vfile) parseYouku() (err error) {

	m.Type = "youku"

	var body string
	var ma []string
	var re *regexp.Regexp

	body, err = curl(m.Url)
	if err != nil {
		return errors.New(fmt.Sprintf("fetch index failed: %v", err))
	}

	re, err = regexp.Compile(`<meta name="title" content="([^"]+)">`)
	ma = re.FindStringSubmatch(body)
	if len(ma) >= 2 {
		m.Desc = "[优酷]"+ma[1]
	}

	re, err = regexp.Compile(`videoId = '([^']+)'`)
	ma = re.FindStringSubmatch(body)

	if len(ma) != 2 {
		return errors.New("youku: cannot find videoId")
	}

	vid := ma[1]
	tms := fmt.Sprintf("%d", time.Now().Unix())
	m3u8url := "http://www.youku.com/player/getM3U8/vid/" + vid + "/type/hd2/ts/" + tms + "/v.m3u8"

	body, err = curl(m3u8url)
	if err != nil {
		return errors.New(fmt.Sprintf("fetch m3u8 failed: %v", err))
	}
	m.m3u8body = body

	return
}

func (m *vfile) dump() {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	ioutil.WriteFile(filepath.Join(m.path, "info"), b, 0777)
}

func (m *vfile) load() (error) {
	b, err := ioutil.ReadFile(filepath.Join(m.path, "info"))
	if err != nil {
		return err
	}
	json.Unmarshal(b, m)
	return nil
}

func (m *vfile) sortUploadM3u8() () {
	_body, err := ioutil.ReadFile(filepath.Join(m.path, "a.m3u8"))
	if err != nil {
		return
	}
	body := string(_body)

	lines := strings.Split(body, "\n")
	var dur float32

	for _, l := range lines {
		if strings.HasPrefix(l, "#EXTINF:") {
			fmt.Sscanf(l, "#EXTINF:%f", &dur)
		}
		if strings.HasPrefix(l, "/vfile") {
			filename := strings.Trim(l, "\r\n/")
			n := len(m.Ts)
			newfilename := filepath.Join(m.path, fmt.Sprintf("%d.ts", n))
			os.Rename(filename, newfilename)
			m.Ts = append(m.Ts, tsinfo{
				Dur: dur,
			})
			dur = 0
		}
	}
	m.downN = len(m.Ts)

	return
}

func (m *vfile) probe() (err error) {
	var info avprobeInfo
	err, info = avprobe(m.Filename)
	if err != nil {
		return err
	}
	m.copyAvprobeInfo(info)
	return
}

func (m *vfile) upload(r io.Reader, length int64, ext string) {
	m.Starttm = time.Now()

	m.l.Lock()
	m.log("upload start")
	m.Type = "upload"
	m.Stat = "uploading"
	m.Size = 0
	m.speed = 0
	m.l.Unlock()

	var err error

	shit := func () {
		m.l.Lock()
		m.log("error: %s", err)
		m.Stat = "error"
		m.err = err
		m.l.Unlock()
	}

	var f *os.File
	filename := filepath.Join(m.path, "0"+ext)
	m.Filename = filename

	f, err = os.Create(filename)
	if err != nil {
		shit()
		return
	}

	tmstart := time.Now()
	var n,ntx int64

	probedNr := 0
	probedOk := false

	doProbe := func () error {
		probedNr++
		var info avprobeInfo
		err, info = avprobe(filename)
		if err != nil {
			return err
		}

		m.copyAvprobeInfo(info)

		probedOk = true
		return nil
	}

	for {
		size := int64(64*1024)
		n, err = io.CopyN(f, r, size)
		if err == io.EOF {
			break
		}
		if err != nil {
			err = errors.New(fmt.Sprintf("when uploading: %v", err))
			shit()
			return
		}

		m.l.Lock()
		m.Size += n
		ntx += n
		since := time.Since(tmstart)
		if since > time.Second {
			tmstart = time.Now()
			m.progress = float32(m.Size)/(float32(length)+1)
			m.speed = ntx*1000/int64(since/time.Millisecond+1)
			ntx = 0
			m.log("progress %.1f%% speed %s/s", m.progress*100, sizestr(m.speed))
		}
		m.l.Unlock()

		if !probedOk && m.Size > int64(probedNr)*1024*512 && probedNr < 40 {
			doProbe()
		}
	}

	err = doProbe()
	if err != nil {
		shit()
		return
	}

	m.l.Lock()
	m.Stat = "conv"
	m.l.Unlock()
	err = avconvM3u8(m.Filename, m.path, func (info avconvInfo) {
		m.conv = info
	})
	if err != nil {
		shit()
		return
	}

	m.sortUploadM3u8()

	m.l.Lock()
	m.log("done")
	m.Stat = "done"
	m.speed = 0
	m.l.Unlock()

	m.dump()
}

func (m *vfile) download(url string) {
	m.Starttm = time.Now()

	for retry := 0; retry < 10; retry++ {
		var err error
		for {
			m.log("download start")
			m.l.Lock()
			m.Stat = "parsing"
			m.l.Unlock()

			switch {
			case strings.HasPrefix(url, "http://v.youku.com"):
				err = m.parseYouku()
			case strings.HasPrefix(url, "http://tv.sohu.com"):
				err = m.parseSohu()
			default:
				return
			}
			if err != nil {
				break
			}

			ioutil.WriteFile(filepath.Join(m.path, "orig.m3u8"), []byte(m.m3u8body), 0777)

			m.l.Lock()
			m.Ts = m.parseM3u8()
			m.Dur = 0
			for _, t := range m.Ts {
				m.Dur += t.Dur
			}
			m.log("m3u8 dur %f", m.Dur)
			m.Stat = "downloading"
			m.progress = 0
			m.downN = 0
			m.l.Unlock()

			err = m.downloadAllTs()
			if err != nil {
				break
			}

			m.l.Lock()
			m.log("done")
			m.Stat = "done"
			m.l.Unlock()

			m.dump()

			return
		}
		m.l.Lock()
		m.log("error: %s", err)
		m.Stat = "error"
		m.err = err
		m.l.Unlock()
		time.Sleep(3*time.Second)
	}
	return
}

func (m *vfile) downloadTs(t *tsinfo, w io.Writer) (err error) {

	var req *http.Request
	req, err = http.NewRequest("GET", t.url, nil)
	if err != nil {
		err = errors.New(fmt.Sprintf("getts: new http req failed %v", err))
		return
	}
	req.Header = http.Header {
		"Accept" : {"*/*"},
		"User-Agent" : {"curl/7.29.0"},
	}

	var resp *http.Response
	tr := &http.Transport {
		DisableCompression: true,
		Dial: func (netw, addr string) (net.Conn, error) {
			return net.DialTimeout(netw, addr, time.Second*40)
		},
	}
	client := &http.Client{
		Transport: tr,
	}
	resp, err = client.Do(req)
	if err != nil {
		err = errors.New(fmt.Sprintf("getts: http get failed %v", err))
		return
	}

	t.Size = 0
	var n,ntx int64
	tmstart := time.Now()

	for {
		size := int64(64*1024)
		n, err = io.CopyN(w, resp.Body, size)
		t.Size += n
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			err = errors.New(fmt.Sprintf("getts: fetch failed %v", err))
			break
		}

		m.l.Lock()
		ntx += n
		m.Size += n

		if resp.ContentLength != -1 {
			m.progress = (m.downDur + float32(t.Size)/float32(resp.ContentLength)*t.Dur) /m.Dur
		} else {
			m.progress = m.downDur/m.Dur
		}

		since := time.Since(tmstart)
		if since > time.Second {
			tmstart = time.Now()
			m.speed = ntx*1000/int64(since/time.Millisecond+1)
			ntx = 0
			m.log("progress %.1f%% speed %s/s", m.progress*100, sizestr(m.speed))
		}
		m.l.Unlock()

	}

	m.l.Lock()
	m.speed = 0
	m.l.Unlock()

	return
}

func (m *vfile) downloadAllTs() (err error) {
	m.dump()
	m.downDur = 0
	for i, t := range m.Ts {
		var w *os.File
		path := filepath.Join(m.path, fmt.Sprintf("%d.ts", i))
		w, err = os.Create(path)
		if err != nil {
			err = errors.New(fmt.Sprintf("getallts: create %s failed", path))
			return
		}

		m.log("downloading ts %d/%d", i+1, len(m.Ts))

		err = m.downloadTs(&t, w)
		if err != nil {
			return
		}
		m.l.Lock()
		m.downDur += t.Dur
		m.downN++
		m.l.Unlock()
		w.Close()

		if i == 0 {
			var info avprobeInfo
			err, info = avprobe(filepath.Join(m.path, "0.ts"))
			if err == nil {
				m.copyAvprobeInfo(info)
				m.log("avprobe: size %dx%d", m.W, m.H)
			} else {
				m.log("avprobe failed: %v", err)
			}
		}
	}
	return
}

func (m *vfile) copyAvprobeInfo(info avprobeInfo) {
	m.W = info.w
	m.H = info.h
	m.Acodec = info.acodec
	m.Vcodec = info.vcodec
	m.Ainfo = info.ainfo
	m.Vinfo = info.vinfo
	m.Fps = info.fps
	m.Bitrate = info.bitrate
	m.Dur = info.dur
}

func (v *vfile) avprobe() {
	for _, s := range []string{"0.ts", "0.mp4"} {
		err, info := avprobe(filepath.Join(v.path, s))
		if err == nil {
			v.copyAvprobeInfo(info)
			break
		}
	}
}

func (v vfile) Statstr() string {
	stat := ""
	switch v.Stat {
	case "parsing":
		stat += "[解析中]"
	case "downloading":
		stat += fmt.Sprintf("[下载中%.1f%%]", v.progress*100)
		stat += fmt.Sprintf("[%s]", sizestr(v.Size))
	case "done":
		stat += "[已完成]"
		stat += fmt.Sprintf("[%s]", sizestr(v.Size))
	case "uploading":
		stat += fmt.Sprintf("[上传中%.1f%%]", v.progress*100)
		stat += fmt.Sprintf("[%s]", sizestr(v.Size))
	case "conv":
		stat += fmt.Sprintf("[转码中%.1f%%]", v.conv.per*100)
		stat += fmt.Sprintf("[%s/s]", sizestr(int64(v.conv.kbps)*1024/8))
		stat += fmt.Sprintf("[%s]", sizestr(v.Size))
	case "error":
		stat += "[出错]"
	case "nonexist":
		stat += "[不存在]"
	}
	if v.Dur > 0.0 {
		stat += fmt.Sprintf("[%s]", durstr(v.Dur))
	}
	if v.W != 0 {
		stat += fmt.Sprintf("[%dx%d]", v.W, v.H)
	}
	return stat
}

func (v *vfile) genLiveEndM3u8(w io.Writer, host string, at float32) {
	pos := float32(0)
	fmt.Fprintf(w, "#EXTM3U\n")
	fmt.Fprintf(w, "#EXT-X-TARGETDURATION:%.0f\n", 10.0)
	for i, t := range v.Ts {
		pos += t.Dur
		if pos > at {
			fmt.Fprintf(w, "#EXTINF:%.0f,\n", t.Dur)
			fmt.Fprintf(w, "http://%s/%s/%d.ts\n", host, v.path, i)
		}
	}
	fmt.Fprintf(w, "#EXT-X-ENDLIST\n")
}

func (v *vfile) genM3u8(w io.Writer, host string) {
	maxdur := float32(0)
	for _, t := range v.Ts {
		if t.Dur > maxdur {
			maxdur = t.Dur
		}
	}
	fmt.Fprintf(w, "#EXTM3U\n")
	fmt.Fprintf(w, "#EXT-X-TARGETDURATION:%.1f\n", maxdur)
	for i, t := range v.Ts {
		if v.Size == 0 {
			continue
		}
		fmt.Fprintf(w, "#EXTINF:%.1f,\n", t.Dur)
		fmt.Fprintf(w, "http://%s/%s/%d.ts\n", host, v.path, i)
	}
	fmt.Fprintf(w, "#EXT-X-DISCONTINUITY\n")
	fmt.Fprintf(w, "#EXT-X-ENDLIST\n")
}

type tsinfo struct {
	Dur float32
	Size int64
	url string
}

type vfile struct {
	Url string
	Desc string
	Type string
	Size int64
	Stat string
	Filename string
	Dur float32
	Ts []tsinfo
	Starttm time.Time

	W,H int
	Ainfo,Vinfo string
	Acodec,Vcodec string
	Fps int
	Bitrate int
	conv avconvInfo

	sha string
	path string
	l *sync.Mutex
	m3u8body string
	speed int64
	downDur float32
	downN int
	progress float32
	err error
}

func testvfile() {
	urls := []string {
		"http://v.youku.com/v_show/id_XMTEwMzc0MjQ=.html?f=19250136",
		/*
		"http://tv.sohu.com/20130506/n375007214.shtml",
		"http://v.youku.com/v_show/id_XNTM1NTgzNjQ4.html?f=19249434",
		"http://tv.sohu.com/20130417/n372981909.shtml",
		"http://tv.sohu.com/20130409/n372077553.shtml",
		"http://tv.sohu.com/20130407/n371829027.shtml",
		"http://tv.sohu.com/20130408/n371935984.shtml",
		"http://tv.sohu.com/20130320/n369601988.shtml",
		*/
	}
	for _, u := range urls {
		global.vfile.download(u)
	}

	time.Sleep(30*time.Second)
}

func (v *vfile) HtmlDownOrView() string {
	if v.Stat == "nonexist" {
		return fmt.Sprintf(`<a target=_blank href="/cgi/?do=downvfile&url=%s">下载</a>`, v.Url)
	} else {
		return fmt.Sprintf(`<a target=_blank href="/vfile/%s">查看</a>`, v.sha)
	}
}

func (v *vfile) Typestr() string {
	switch v.Type {
	case "upload":
		return "用户上传"
	case "youku":
		return "优酷下载"
	case "sohu":
		return "搜狐下载"
	}
	return ""
}

func listvfile (m *vfilelist) string {
	return mustache.RenderFile("tpl/listVfile.html",
	map[string]interface{} {
		"list": m.m,
		"statstr": m.statstr(),
	})
}

func vfilePage (w http.ResponseWriter, r *http.Request, path string) {
	v := global.vfile.shotsha(getsha1(path))
	if v != nil {
		http.Redirect(w, r, fmt.Sprintf("/vfile/%s", getsha1(path)), 302)
		return
	}
	v = global.vfile.shotsha(path)
	if v == nil {
		return
	}

	infostr := ""
	hasInfostr := false
	if v.Bitrate != 0 || v.Vinfo != "" || v.Ainfo != "" {
		infostr += fmt.Sprintf("bitrate: %d kb/s\n", v.Bitrate)
		infostr += v.Vinfo + "\n"
		infostr += v.Ainfo + "\n"
		hasInfostr = true
	}

	livenr := global.user.countPlayers("/vfile")
	log.Printf("%s", v.Type=="upload")

	renderIndex(w, "manv",
	mustache.RenderFile("tpl/viewVfile.html", map[string]interface{} {
		"url": v.Url,
		"desc": v.Desc,
		"statstr": v.Statstr(),
		"path": path,
		"starttm": v.Starttm,
		"progress": fmt.Sprintf("%.1f%%", v.progress*100),
		"speed": fmt.Sprintf("%s/s", sizestr(v.speed)),

		"livenr": livenr,

		"hasTsinfo": len(v.Ts) > 0,
		"tsTotal": len(v.Ts),
		"tsDown": v.downN,

		"isUploading": v.Stat == "uploading",
		"isDownloading": v.Stat == "downloading",
		"isError": v.Stat == "error",
		"typeIsUpload": v.Type == "upload",
		"origfile": v.Filename,

		"typestr": v.Typestr(),

		"hasM3u8": v.Type != "upload" || v.Stat == "done",

		"error": fmt.Sprintf("%v", v.err),

		"hasInfostr": hasInfostr,
		"infostr": infostr,
		"isDone": v.Stat == "done",
	}))
}

func manvfilePage(w io.Writer, path string) {
	list := global.vfile.shotall()
	s := listvfile(list)
	renderIndex(w, "manv", s)
}

func editVfilePage(w http.ResponseWriter, r *http.Request, path string) {
	v := global.vfile.shotsha(path)
	if v == nil {
		return
	}
	renderIndex(w, "manv",
	mustache.RenderFile("tpl/editVfilePage.html", map[string]interface{} {
		"title": "编辑视频",
		"desc": v.Desc,
		"path": path,
	}))
}

func doEditVfilePage(w http.ResponseWriter, r *http.Request) {
	path := r.FormValue("path")
	v := global.vfile.m[path]
	if v == nil {
		return
	}
	desc := r.FormValue("desc")
	if desc == "" {
		renderIndex(w, "manv",
		mustache.RenderFile("tpl/alert.html", map[string]interface{} {
			"msg": "标题能为空",
			"back": "/edit/vfile/"+path,
		}))
		return
	}
	v.Desc = desc
	http.Redirect(w, r, "/vfile/"+path, 302)
}

func vfileUpload (w io.Writer, path string) {
	renderIndex(w, "upload", mustache.RenderFile("tpl/vfileUpload.html", map[string]interface{}{}))
}

