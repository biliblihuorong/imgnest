package lsky

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/storage"
)

// adoptSamples is how many images are probed to find a strategy's files.
const adoptSamples = 5

// CheckAdoption makes sure an in-place import will find the strategy's
// originals in the rule's storage and keep their links: it probes a few
// pathnames, and when they are missing it looks for them under each trailing
// part of the strategy URL's path (a CDN often maps
// https://img.example.com/folder/x to key folder/x), so it can name the
// storage folder to set. The storage's Base URL must have the same path as
// the strategy URL; only the scheme and host may change. strategyID 0 means
// the strategy of the first image; folder is the storage's current folder.
func CheckAdoption(ctx context.Context, snap Snapshot, strategyID int64, baseURL, folder string, open func(context.Context, string) (io.ReadCloser, storage.ObjectInfo, error)) error {
	if strategyID == 0 && len(snap.Images) > 0 {
		strategyID = snap.Images[0].StrategyID
	}
	var strategy Strategy
	for _, s := range snap.Strategies {
		if s.ID == strategyID {
			strategy = s
		}
	}
	var samples []string
	for _, image := range snap.Images {
		if image.StrategyID == strategyID && image.Pathname != "" {
			samples = append(samples, image.Pathname)
			if len(samples) == adoptSamples {
				break
			}
		}
	}
	if len(samples) == 0 {
		return nil
	}
	oldPath := urlPath(strategy.URL)
	oldLink := strings.TrimRight(strategy.URL, "/") + "/" + samples[0]
	found, err := anyExists(ctx, open, "", samples)
	if err != nil {
		return err
	}
	if !found {
		parts := strings.Split(oldPath, "/")
		for i := range parts {
			extra := strings.Join(parts[i:], "/")
			if extra == "" {
				continue
			}
			ok, err := anyExists(ctx, open, extra+"/", samples)
			if err != nil {
				return err
			}
			if ok {
				want := strings.Trim(folder+"/"+extra, "/")
				return fmt.Errorf("蓝空的文件在桶里的 %s 文件夹下。请在「存储管理」里把这个存储的「路径前缀」设成 %s，Base URL 保持 %s，再开始导入", want, want, origin(baseURL)+pathOrEmpty(oldPath))
			}
		}
		where := "桶的根目录"
		if folder != "" {
			where = "桶里的 " + folder + " 文件夹"
		}
		return fmt.Errorf("在%s下找不到 %s。请确认存储填的桶名就是蓝空用的那个；如果文件放在桶里的某个文件夹下，在存储里填上「路径前缀」", where, samples[0])
	}
	if urlPath(baseURL) != oldPath {
		return fmt.Errorf("旧链接是 %s，按现在的设置 ImgNest 会生成 %s/%s。请把这个存储的 Base URL 改成 %s 后再开始", oldLink, strings.TrimRight(baseURL, "/"), samples[0], origin(baseURL)+pathOrEmpty(oldPath))
	}
	return nil
}

func anyExists(ctx context.Context, open func(context.Context, string) (io.ReadCloser, storage.ObjectInfo, error), prefix string, samples []string) (bool, error) {
	failed := false
	for _, pathname := range samples {
		body, _, err := open(ctx, prefix+pathname)
		switch {
		case err == nil:
			_ = body.Close()
			return true, nil
		case ctx.Err() != nil:
			return false, ctx.Err()
		case !errors.Is(err, storage.ErrNotFound):
			failed = true
		}
	}
	if failed {
		return false, errors.New("读取存储桶失败，请先在「存储管理」里测试这个存储的连接")
	}
	return false, nil
}

// urlPath is the unescaped path of raw without surrounding slashes.
func urlPath(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.Trim(parsed.Path, "/")
}

func origin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return strings.TrimRight(raw, "/")
	}
	return parsed.Scheme + "://" + parsed.Host
}

func pathOrEmpty(path string) string {
	if path == "" {
		return ""
	}
	return "/" + path
}
