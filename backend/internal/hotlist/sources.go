package hotlist

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"slate/internal/fetch"
)

var sources = map[string]source{
	"zhihu": jsonList("https://api.zhihu.com/topstory/hot-lists/total?limit=50", fetch.Options{}, "data", func(v any) item {
		u := str(v, "target.url")
		if i := strings.LastIndex(u, "/"); i >= 0 && u[i+1:] != "" {
			u = "https://www.zhihu.com/question/" + u[i+1:]
		}
		return item{str(v, "target.title"), u}
	}),
	"weibo": jsonList("https://weibo.com/ajax/side/hotSearch", headers("Referer", "https://weibo.com/"), "data.realtime", func(v any) item {
		t := firstStr(v, "word", "word_scheme")
		return item{t, "https://s.weibo.com/weibo?q=" + url.QueryEscape(t)}
	}),
	"baidu": func(ctx context.Context) ([]item, error) {
		html, err := getText(ctx, "https://top.baidu.com/board?tab=realtime", fetch.Options{})
		if err != nil {
			return nil, err
		}
		data := jsonFromHTML(html, re(`(?s)<!--s-data:(.*?)-->`))
		content := list(data, "data.cards.0.content")
		if content == nil {
			content = list(data, "cards.0.content")
		}
		if nested := list(content, "0.content"); nested != nil {
			content = nested
		}
		return mapItems(content, "", func(v any) item {
			t := firstStr(v, "word", "title")
			return item{t, or(firstStr(v, "rawUrl", "url"), "https://www.baidu.com/s?wd="+url.QueryEscape(or(str(v, "query"), t)))}
		}), nil
	},
	"v2ex": jsonList("https://www.v2ex.com/api/topics/hot.json", fetch.Options{}, "", func(v any) item {
		return item{str(v, "title"), str(v, "url")}
	}),
	"bilibili": jsonList("https://api.bilibili.com/x/web-interface/popular?ps=50&pn=1", headers("Referer", "https://www.bilibili.com/"), "data.list", biliVideo),
	"toutiao": jsonList("https://www.toutiao.com/hot-event/hot-board/?origin=toutiao_pc", fetch.Options{}, "data", func(v any) item {
		return item{str(v, "Title"), ifSet(str(v, "ClusterIdStr"), "https://www.toutiao.com/trending/%s/")}
	}),
	"thepaper": jsonList("https://cache.thepaper.cn/contentapi/wwwIndex/rightSidebar", fetch.Options{}, "data.hotNews", func(v any) item {
		return item{str(v, "name"), ifSet(str(v, "contId"), "https://www.thepaper.cn/newsDetail_forward_%s")}
	}),
	"douyin": func(ctx context.Context) ([]item, error) {
		v, err := getJSON(ctx, "https://aweme.snssdk.com/aweme/v1/hot/search/list/", fetch.Options{})
		if err != nil {
			return nil, err
		}
		path := "data.word_list"
		if list(v, path) == nil {
			path = "word_list"
		}
		return mapItems(v, path, func(v any) item {
			return item{str(v, "word"), ifSet(firstStr(v, "sentence_id", "group_id"), "https://www.douyin.com/hot/%s")}
		}), nil
	},
	"kuaishou": kuaishou,
	"hupu": jsonList("https://m.hupu.com/api/v2/bbs/topicThreads?topicId=1&page=1", fetch.Options{}, "data.topicThreads", func(v any) item {
		return item{str(v, "title"), or(ifSet(str(v, "tid"), "https://bbs.hupu.com/%s.html"), str(v, "url"))}
	}),
	"tieba": jsonList("https://tieba.baidu.com/hottopic/browse/topicList", fetch.Options{}, "data.bang_topic.topic_list", func(v any) item {
		return item{str(v, "topic_name"), str(v, "topic_url")}
	}),
	"juejin": jsonList("https://api.juejin.cn/content_api/v1/content/article_rank?category_id=1&type=hot", headers("Referer", "https://juejin.cn/hot/articles"), "data", func(v any) item {
		return item{str(v, "content.title"), ifSet(str(v, "content.content_id"), "https://juejin.cn/post/%s")}
	}),
	"sspai": jsonList("https://sspai.com/api/v1/article/tag/page/get?limit=40&tag=%E7%83%AD%E9%97%A8%E6%96%87%E7%AB%A0", fetch.Options{}, "data", func(v any) item {
		return item{str(v, "title"), ifSet(str(v, "id"), "https://sspai.com/post/%s")}
	}),
	"ithome":          ithome,
	"smzdm":           smzdm,
	"36kr":            kr36,
	"github-trending": githubTrending,
	"dongchedi": func(ctx context.Context) ([]item, error) {
		html, err := getText(ctx, "https://www.dongchedi.com/news", headers("Referer", "https://www.dongchedi.com/"))
		if err != nil {
			return nil, err
		}
		data := jsonFromHTML(html, re(`(?is)<script[^>]*id="__NEXT_DATA__"[^>]*>(.*?)</script>`))
		return mapItems(data, "props.pageProps.hotSearchList", func(v any) item {
			t := str(v, "title")
			return item{t, ifSet(url.QueryEscape(t), "https://www.dongchedi.com/search?keyword=%s")}
		}), nil
	},
	"hello-github": jsonList("https://api.hellogithub.com/v1/?sort_by=featured&page=1&rank_by=newest&tid=all", fetch.Options{}, "data", func(v any) item {
		title := str(v, "name")
		if t := str(v, "title"); t != "" {
			title = strings.Trim(title+" - "+t, " -")
		}
		return item{title, ifSet(str(v, "full_name"), "https://hellogithub.com/repository/%s")}
	}),
	"netease": jsonList("https://m.163.com/fe/api/hot/news/flow", fetch.Options{}, "data.list", func(v any) item {
		return item{str(v, "title"), or(ifSet(firstStr(v, "skipID", "docid"), "https://www.163.com/dy/article/%s.html"), str(v, "url"))}
	}),
	"netease-music": func(ctx context.Context) ([]item, error) {
		v, err := getJSON(ctx, "https://music.163.com/api/v6/playlist/detail?id=3778678&n=1000", headers("authority", "music.163.com", "Referer", "https://music.163.com/"))
		if err != nil {
			return nil, err
		}
		path := "playlist.tracks"
		if list(v, path) == nil {
			path = "result.tracks"
		}
		return mapItems(v, path, func(v any) item {
			return item{str(v, "name"), ifSet(str(v, "id"), "https://music.163.com/#/song?id=%s")}
		}), nil
	},
	"qq": qqNews("https://r.inews.qq.com/gw/event/hot_ranking_list", headers("Referer", "https://news.qq.com/")),
	"quark": jsonList("https://iflow.quark.cn/iflow/api/v1/article/aggregation?aggregation_id=16665090098771297825&count=50&bottom_pos=0", headers("Referer", "https://iflow.quark.cn/"), "data.articles", func(v any) item {
		return item{str(v, "title"), ifSet(str(v, "id"), "https://123.quark.cn/detail?item_id=%s")}
	}),
	"woshipm": jsonList("https://www.woshipm.com/api2/app/article/popular/daily", headers("Referer", "https://www.woshipm.com/"), "RESULT", func(v any) item {
		u := ""
		if id, typ := str(v, "data.id"), str(v, "data.type"); id != "" && typ != "" {
			u = fmt.Sprintf("https://www.woshipm.com/%s/%s.html", typ, id)
		}
		return item{str(v, "data.articleTitle"), u}
	}),

	"51cto":   cto51,
	"52pojie": rss("https://www.52pojie.cn/forum.php?mod=guide&view=digest&rss=1", true, headers("User-Agent", mobileUA)),
	"acfun": jsonList("https://www.acfun.cn/rest/pc-direct/rank/channel?channelId=&rankLimit=30&rankPeriod=DAY", headers("Referer", "https://www.acfun.cn/rank/list/?cid=-1&pcid=-1&range=DAY"), "rankList", func(v any) item {
		return item{str(v, "contentTitle"), ifSet(str(v, "dougaId"), "https://www.acfun.cn/v/ac%s")}
	}),
	"csdn": jsonList("https://blog.csdn.net/phoenix/web/blog/hot-rank?page=0&pageSize=30", fetch.Options{}, "data", func(v any) item {
		return item{str(v, "articleTitle"), str(v, "articleDetailUrl")}
	}),
	"dgtle": jsonList("https://opser.api.dgtle.com/v2/news/index", fetch.Options{}, "items", func(v any) item {
		u := ""
		if id := str(v, "id"); id != "" {
			u = fmt.Sprintf("https://www.dgtle.com/news-%s-%s.html", id, str(v, "type"))
		}
		return item{firstStr(v, "title", "content"), u}
	}),
	"douban-group": htmlList("https://www.douban.com/group/explore", fetch.Options{}, re(`(?is)<div[^>]*class="[^"]*channel-item[^"]*"[^>]*>(.*?)</div>\s*</div>`), func(b string) item {
		return item{first(b, re(`(?is)<h3[^>]*>\s*<a[^>]*>(.*?)</a>`)), first(b, re(`(?is)<h3[^>]*>\s*<a[^>]+href="([^"]+)"`))}
	}),
	"douban-movie": htmlList("https://movie.douban.com/chart/", headers("User-Agent", mobileUA), re(`(?is)<tr[^>]*class="[^"]*item[^"]*"[^>]*>(.*?)</tr>`), func(b string) item {
		title := first(b, re(`(?i)<a[^>]+title="([^"]+)"`))
		if score := first(b, re(`(?is)<span[^>]*class="[^"]*rating_nums[^"]*"[^>]*>(.*?)</span>`)); score != "" {
			title = "【" + score + "】" + title
		}
		return item{title, first(b, re(`(?i)<a[^>]+href="([^"]+)"`))}
	}),
	"gameres": htmlList("https://www.gameres.com", fetch.Options{}, re(`(?is)<article[^>]*class="[^"]*feed-item[^"]*"[^>]*>(.*?)</article>`), func(b string) item {
		return item{first(b, re(`(?is)<a[^>]*class="[^"]*feed-item-title-a[^"]*"[^>]*>(.*?)</a>`)),
			absURL("https://www.gameres.com", first(b, re(`(?i)<a[^>]*class="[^"]*feed-item-title-a[^"]*"[^>]*href="([^"]+)"`)))}
	}),
	"geekpark": jsonList("https://mainssl.geekpark.net/api/v2", fetch.Options{}, "homepage_posts", func(v any) item {
		return item{str(v, "post.title"), ifSet(str(v, "post.id"), "https://www.geekpark.net/news/%s")}
	}),
	"guokr": jsonList("https://www.guokr.com/beta/proxy/science_api/articles?limit=30", fetch.Options{}, "", func(v any) item {
		return item{str(v, "title"), ifSet(str(v, "id"), "https://www.guokr.com/article/%s")}
	}),
	"hackernews": htmlList("https://news.ycombinator.com", fetch.Options{}, re(`(?is)<tr[^>]*class="[^"]*\bathing\b[^"]*"[^>]*>(.*?)</tr>`), func(b string) item {
		return item{first(b, re(`(?is)<span[^>]*class="[^"]*titleline[^"]*"[^>]*>\s*<a[^>]*>(.*?)</a>`)),
			absURL("https://news.ycombinator.com/", first(b, re(`(?is)<span[^>]*class="[^"]*titleline[^"]*"[^>]*>\s*<a[^>]+href="([^"]+)"`)))}
	}),
	"huxiu": jsonList("https://moment-api.huxiu.com/web-v3/moment/feed?platform=www", headers("Referer", "https://www.huxiu.com/moment/"), "data.moment_list.datalist", func(v any) item {
		content := re(`(?i)<br\s*/?>`).ReplaceAllString(str(v, "content"), "\n")
		title := ""
		for _, line := range strings.Split(content, "\n") {
			if title = fetch.StripHTML(line); title != "" {
				break
			}
		}
		return item{title, ifSet(str(v, "object_id"), "https://www.huxiu.com/moment/%s.html")}
	}),
	"ifanr": jsonList("https://sso.ifanr.com/api/v5/wp/buzz/?limit=20&offset=0", fetch.Options{}, "objects", func(v any) item {
		return item{str(v, "post_title"), or(str(v, "buzz_original_url"), ifSet(str(v, "post_id"), "https://www.ifanr.com/%s"))}
	}),
	"ithome-xijiayi": htmlList("https://www.ithome.com/zt/xijiayi", fetch.Options{}, re(`(?is)<li\b[^>]*>(.*?)</li>`), func(b string) item {
		return item{first(b, re(`(?is)<h2[^>]*>(.*?)</h2>`)), first(b, re(`(?i)<a[^>]+href="([^"]+)"`))}
	}),
	"jianshu": func(ctx context.Context) ([]item, error) {
		v, err := getJSON(ctx, "https://www.jianshu.com/asimov/trending/now", headers("Accept", "application/json, text/plain, */*", "Referer", "https://www.jianshu.com/"))
		if err != nil {
			return nil, err
		}
		var out []item
		for _, e := range list(v, "") {
			if num(e, "object.type") == 1 {
				out = append(out, item{str(e, "object.data.title"), ifSet(str(e, "object.data.slug"), "https://www.jianshu.com/p/%s")})
			}
		}
		return out, nil
	},
	"linuxdo": rss("https://linux.do/top.rss?period=weekly", false, fetch.Options{}),
	"miyoushe": jsonList("https://bbs-api-static.miyoushe.com/painter/wapi/getNewsList?client_type=4&gids=5&last_id=&page_size=30&type=1", fetch.Options{}, "data.list", func(v any) item {
		return item{str(v, "post.subject"), ifSet(str(v, "post.post_id"), "https://www.miyoushe.com/ys/article/%s")}
	}),
	"netease-news": jsonList("https://m.163.com/fe/api/hot/news/flow", fetch.Options{}, "data.list", func(v any) item {
		return item{str(v, "title"), ifSet(str(v, "docid"), "https://www.163.com/dy/article/%s.html")}
	}),
	"newsmth": jsonList("https://wap.newsmth.net/wap/api/hot/global", fetch.Options{}, "data.topics", func(v any) item {
		u := ""
		if id := str(v, "article.topicId"); id != "" {
			u = fmt.Sprintf("https://wap.newsmth.net/article/%s?title=%s&from=home", id, url.QueryEscape(str(v, "board.title")))
		}
		return item{str(v, "article.subject"), u}
	}),
	"ngabbs": func(ctx context.Context) ([]item, error) {
		o := headers("Accept", "*/*", "Referer", "https://ngabbs.com/", "X-User-Agent", "NGA_skull/7.3.1(iPhone13,2;iOS 17.2.1)")
		o.Method, o.Body = "POST", url.Values{"__output": {"14"}}
		v, err := getJSON(ctx, "https://ngabbs.com/nuke.php?__lib=load_topic&__act=load_topic_reply_ladder2&opt=1&all=1", o)
		if err != nil {
			return nil, err
		}
		return mapItems(v, "result.0", func(v any) item {
			return item{str(v, "subject"), ifSet(str(v, "tpcurl"), "https://bbs.nga.cn%s")}
		}), nil
	},
	"nodeseek": rss("https://www.nodeseek.com/rss.xml", false, fetch.Options{}),
	"nytimes":  rss("https://cn.nytimes.com/rss/", false, headers("User-Agent", mobileUA)),
	"producthunt": htmlList("https://www.producthunt.com/feed", fetch.Options{}, re(`(?is)<entry\b[^>]*>(.*?)</entry>`), func(b string) item {
		return item{first(b, re(`(?is)<title\b[^>]*>(.*?)</title>`)), first(b, re(`(?i)<link\b[^>]*rel="alternate"[^>]*href="([^"]+)"`))}
	}),
	"qq-news": qqNews("https://r.inews.qq.com/gw/event/hot_ranking_list?page_size=50", fetch.Options{}),
	"sina": jsonList("https://newsapp.sina.cn/api/hotlist?newsId=HB-1-snhs%2Ftop_news_list-all", fetch.Options{}, "data.hotList", func(v any) item {
		return item{str(v, "info.title"), str(v, "base.base.url")}
	}),
	"sina-news": func(ctx context.Context) ([]item, error) {
		ymd := time.Now().In(cst).Format("20060102")
		text, err := getText(ctx, "https://top.news.sina.com.cn/ws/GetTopDataList.php?top_type=day&top_cat=www_www_all_suda_suda&top_time="+ymd+"&top_show_num=50", fetch.Options{})
		if err != nil {
			return nil, err
		}
		var v any
		if err := json.Unmarshal([]byte(balancedObject(text)), &v); err != nil {
			return nil, fmt.Errorf("新浪新闻 JSONP 解析失败: %w", err)
		}
		return mapItems(v, "data", func(v any) item { return item{str(v, "title"), str(v, "url")} }), nil
	},
	"weread": jsonList("https://weread.qq.com/web/bookListInCategory/rising?rank=1", fetch.Options{}, "books", func(v any) item {
		return item{str(v, "bookInfo.title"), ""}
	}),
	"yystv": jsonList("https://www.yystv.cn/home/get_home_docs_by_page", headers("Accept", "application/json,text/html,*/*", "Referer", "https://www.yystv.cn/docs", "X-Requested-With", "XMLHttpRequest"), "data", func(v any) item {
		return item{str(v, "title"), ifSet(str(v, "id"), "https://www.yystv.cn/p/%s")}
	}),
	"zhihu-daily": zhihuDaily,

	"36kr-quick": htmlList("https://www.36kr.com/newsflashes", fetch.Options{}, re(`(?is)<div[^>]*class="[^"]*newsflash-item[^"]*"[^>]*>(.*?)</div>\s*</div>`), func(b string) item {
		return item{first(b, re(`(?is)<a[^>]*class="[^"]*item-title[^"]*"[^>]*>(.*?)</a>`)),
			absURL("https://www.36kr.com", first(b, re(`(?i)<a[^>]*class="[^"]*item-title[^"]*"[^>]*href="([^"]+)"`)))}
	}),
	"bilibili-hot-search": jsonList("https://s.search.bilibili.com/main/hotword?limit=30", fetch.Options{}, "list", func(v any) item {
		return item{firstStr(v, "show_name", "keyword"), ifSet(url.QueryEscape(str(v, "keyword")), "https://search.bilibili.com/all?keyword=%s")}
	}),
	"bilibili-ranking": jsonList("https://api.bilibili.com/x/web-interface/popular?ps=50&pn=1", headers("Referer", "https://www.bilibili.com/v/popular/rank/all"), "data.list", biliVideo),
	"cankaoxiaoxi":     cankaoxiaoxi,
	"douban": jsonList("https://m.douban.com/rexxar/api/v2/subject/recent_hot/movie", headers("Referer", "https://movie.douban.com/", "Accept", "application/json, text/plain, */*"), "items", func(v any) item {
		return item{str(v, "title"), ifSet(str(v, "id"), "https://movie.douban.com/subject/%s")}
	}),
	"chongbuluo-hot": htmlList("https://www.chongbuluo.com/forum.php?mod=guide&view=hot", fetch.Options{}, re(`(?is)<tr\b[^>]*>(.*?)</tr>`), func(b string) item {
		return item{first(b, re(`(?is)<a[^>]*class="[^"]*xst[^"]*"[^>]*>(.*?)</a>`)),
			absURL("https://www.chongbuluo.com/", first(b, re(`(?i)<a[^>]*class="[^"]*xst[^"]*"[^>]*href="([^"]+)"`)))}
	}),
	"chongbuluo-latest": rss("https://www.chongbuluo.com/forum.php?mod=rss&view=newthread", false, fetch.Options{}),
	"cls-depth": func(ctx context.Context) ([]item, error) {
		v, err := getJSON(ctx, "https://www.cls.cn/v3/depth/home/assembled/1000?"+clsQuery(), fetch.Options{})
		if err != nil {
			return nil, err
		}
		rows := list(v, "data.depth_list")
		sort.SliceStable(rows, func(i, j int) bool { return num(rows[i], "ctime") > num(rows[j], "ctime") })
		return mapItems(rows, "", clsItem), nil
	},
	"cls-hot": jsonList("https://www.cls.cn/v2/article/hot/list?"+clsQuery(), fetch.Options{}, "data", clsItem),
	"cls-telegraph": func(ctx context.Context) ([]item, error) {
		v, err := getJSON(ctx, "https://www.cls.cn/v1/roll/get_roll_list?"+clsRollQuery(), headers("Accept", "application/json, text/plain, */*", "Referer", "https://www.cls.cn/telegraph"))
		if err != nil {
			return nil, err
		}
		var out []item
		for _, e := range list(v, "data.roll_data") {
			if num(e, "is_ad") == 0 {
				out = append(out, clsItem(e))
			}
		}
		return out, nil
	},
	"fastbull":         fastbull,
	"fastbull-express": fastbull,
	"fastbull-news": func(ctx context.Context) ([]item, error) {
		html, err := getText(ctx, "https://www.fastbull.com/cn/news", fetch.Options{})
		if err != nil {
			return nil, err
		}
		var out []item
		for _, m := range re(`(?is)<a[^>]*href="([^"]+)"[^>]*class="[^"]*trending_type[^"]*"[^>]*>.*?<h4[^>]*class="[^"]*title[^"]*"[^>]*>(.*?)</h4>`).FindAllStringSubmatch(html, -1) {
			out = append(out, item{m[2], absURL("https://www.fastbull.com", m[1])})
		}
		return out, nil
	},
	"freebuf": func(ctx context.Context) ([]item, error) {
		html, err := getText(ctx, "https://www.freebuf.com/", headers("Referer", "https://www.freebuf.com/"))
		if err != nil {
			return nil, err
		}
		var out []item
		for _, m := range re(`(?s)(?:ID|id):"?(\d+)"?,post_title:"([^"]+)".{0,500}?url:"([^"]+)"`).FindAllStringSubmatch(html, -1) {
			out = append(out, item{m[2], absURL("https://www.freebuf.com", strings.ReplaceAll(m[3], `/`, "/"))})
		}
		return out, nil
	},
	"gelonghui": func(ctx context.Context) ([]item, error) {
		html, err := getText(ctx, "https://www.gelonghui.com/news/", fetch.Options{})
		if err != nil {
			return nil, err
		}
		var out []item
		for _, m := range re(`(?is)<section[^>]*class="article-content".*?<section[^>]*class="detail-right".*?<a[^>]*href="([^"]+)".*?<h2[^>]*>(.*?)</h2>`).FindAllStringSubmatch(html, -1) {
			out = append(out, item{m[2], absURL("https://www.gelonghui.com", m[1])})
		}
		return out, nil
	},
	"ifeng": func(ctx context.Context) ([]item, error) {
		html, err := getText(ctx, "https://www.ifeng.com/", fetch.Options{})
		if err != nil {
			return nil, err
		}
		m := re(`(?s)var\s+allData\s*=\s*(\{.*?\});`).FindStringSubmatch(html)
		var v any
		if m == nil || json.Unmarshal([]byte(m[1]), &v) != nil {
			return nil, fmt.Errorf("凤凰网页面结构变更")
		}
		return mapItems(v, "hotNews1", func(v any) item { return item{str(v, "title"), str(v, "url")} }), nil
	},
	"iqiyi-hot-ranklist": jsonList("https://mesh.if.iqiyi.com/portal/lw/v7/channel/card/videoTab?channelName=recommend&data_source=v7_rec_sec_hot_rank_list&tempId=85&count=30&block_id=hot_ranklist&device=14a4b5ba98e790dce6dc07482447cf48&from=webapp", headers("Referer", "https://www.iqiyi.com"), "items.0.video.0.data", func(v any) item {
		return item{firstStr(v, "title", "display_name"), str(v, "page_url")}
	}),
	"jin10": func(ctx context.Context) ([]item, error) {
		text, err := getText(ctx, fmt.Sprintf("https://www.jin10.com/flash_newest.js?t=%d", time.Now().UnixMilli()), fetch.Options{})
		if err != nil {
			return nil, err
		}
		raw := strings.TrimRight(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "var newest = ")), ";")
		if i := strings.IndexByte(raw, '['); i >= 0 {
			raw = raw[i:]
		}
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, fmt.Errorf("金十数据解析失败: %w", err)
		}
		var out []item
		for _, e := range list(v, "") {
			text := fetch.StripHTML(firstStr(e, "data.title", "data.content"))
			skip := text == ""
			for _, ch := range list(e, "channel") {
				skip = skip || ch == float64(5)
			}
			if skip {
				continue
			}
			if m := re(`^【([^】]*)】`).FindStringSubmatch(text); m != nil {
				text = m[1]
			}
			out = append(out, item{text, ifSet(str(e, "id"), "https://flash.jin10.com/detail/%s")})
		}
		return out, nil
	},
	"kaopu": func(ctx context.Context) ([]item, error) {
		v, err := getJSON(ctx, "https://kaopustorage.blob.core.windows.net/news-prod/news_list_hans_0.json", fetch.Options{})
		if err != nil {
			return nil, err
		}
		var out []item
		for _, e := range list(v, "") {
			if p := str(e, "publisher"); p != "财新" && p != "公视" {
				out = append(out, item{str(e, "title"), str(e, "link")})
			}
		}
		return out, nil
	},
	"mktnews-flash": func(ctx context.Context) ([]item, error) {
		v, err := getJSON(ctx, "https://api.mktnews.net/api/flash?type=0&limit=50", fetch.Options{})
		if err != nil {
			return nil, err
		}
		rows := list(v, "data")
		sort.SliceStable(rows, func(i, j int) bool { return str(rows[i], "time") > str(rows[j], "time") })
		return mapItems(rows, "", func(v any) item {
			title := str(v, "data.title")
			if title == "" {
				content := str(v, "data.content")
				if m := re(`^【([^】]*)】`).FindStringSubmatch(content); m != nil {
					content = m[1]
				}
				title = content
			}
			return item{title, ifSet(str(v, "id"), "https://mktnews.net/flashDetail.html?id=%s")}
		}), nil
	},
	"nowcoder": func(ctx context.Context) ([]item, error) {
		v, err := getJSON(ctx, fmt.Sprintf("https://gw-c.nowcoder.com/api/sparta/hot-search/top-hot-pc?size=20&_=%d&t=", time.Now().UnixMilli()), fetch.Options{})
		if err != nil {
			return nil, err
		}
		return mapItems(v, "data.result", func(v any) item {
			u := ifSet(str(v, "id"), "https://www.nowcoder.com/discuss/%s")
			if num(v, "type") == 74 && str(v, "uuid") != "" {
				u = "https://www.nowcoder.com/feed/main/detail/" + str(v, "uuid")
			}
			return item{str(v, "title"), u}
		}), nil
	},
	"pcbeta-windows11": rss("https://bbs.pcbeta.com/forum.php?mod=rss&fid=563&auth=0", false, fetch.Options{}),
	"solidot": htmlList("https://www.solidot.org", fetch.Options{}, re(`(?is)<div[^>]*class="[^"]*block_m[^"]*"[^>]*>(.*?)</div>\s*</div>`), func(b string) item {
		return item{first(b, re(`(?is)<div[^>]*class="[^"]*bg_htit[^"]*"[^>]*>.*<a[^>]*>(.*?)</a>`)),
			absURL("https://www.solidot.org", first(b, re(`(?is)<div[^>]*class="[^"]*bg_htit[^"]*"[^>]*>.*<a[^>]+href="([^"]+)"`)))}
	}),
	"sputniknewscn": htmlList("https://sputniknews.cn/services/widget/lenta/", fetch.Options{}, re(`(?is)<div[^>]*class="[^"]*lenta__item[^"]*"[^>]*>(.*?)</div>`), func(b string) item {
		return item{first(b, re(`(?is)<span[^>]*class="[^"]*lenta__item-text[^"]*"[^>]*>(.*?)</span>`)),
			absURL("https://sputniknews.cn", first(b, re(`(?i)<a[^>]+href="([^"]+)"`)))}
	}),
	"steam": htmlList("https://store.steampowered.com/stats/stats/", fetch.Options{}, re(`(?is)<tr[^>]*class="[^"]*player_count_row[^"]*"[^>]*>(.*?)</tr>`), func(b string) item {
		return item{first(b, re(`(?is)<a[^>]*class="[^"]*gameLink[^"]*"[^>]*>(.*?)</a>`)), first(b, re(`(?i)<a[^>]*class="[^"]*gameLink[^"]*"[^>]*href="([^"]+)"`))}
	}),
	"tencent-hot": jsonList("https://i.news.qq.com/web_backend/v2/getTagInfo?tagId=aEWqxLtdgmQ%3D", headers("Referer", "https://news.qq.com/"), "data.tabs.0.articleList", func(v any) item {
		return item{str(v, "title"), str(v, "link_info.url")}
	}),
	"v2ex-share":         v2exShare,
	"wallstreetcn-hot":   jsonList("https://api-one.wallstcn.com/apiv1/content/articles/hot?period=all", fetch.Options{}, "data.day_items", wallstreetItem),
	"wallstreetcn-quick": jsonList("https://api-one.wallstcn.com/apiv1/content/lives?channel=global-channel&limit=30", fetch.Options{}, "data.items", wallstreetItem),
	"wallstreetcn-news": func(ctx context.Context) ([]item, error) {
		v, err := getJSON(ctx, "https://api-one.wallstcn.com/apiv1/content/information-flow?channel=global-channel&accept=article&limit=30", fetch.Options{})
		if err != nil {
			return nil, err
		}
		var out []item
		for _, e := range list(v, "data.items") {
			if t := str(e, "resource_type"); t != "theme" && t != "ad" && str(e, "resource.type") != "live" {
				out = append(out, wallstreetItem(at(e, "resource")))
			}
		}
		return out, nil
	},
	"xueqiu-hotstock": xueqiu,
	"zaobao": func(ctx context.Context) ([]item, error) {
		html, err := getGBK(ctx, "https://www.zaochenbao.com/realtime/", fetch.Options{})
		if err != nil {
			return nil, err
		}
		var out []item
		for _, m := range re(`(?is)<a[^>]*href="([^"]+)"[^>]*class="[^"]*item[^"]*"[^>]*>(.*?)</a>`).FindAllStringSubmatch(html, -1) {
			out = append(out, item{first(m[2], re(`(?is)<div[^>]*class="[^"]*eps[^"]*"[^>]*>(.*?)</div>`)), absURL("https://www.zaochenbao.com", m[1])})
		}
		return out, nil
	},
	"qqvideo-tv-hotsearch": qqVideo,
}

var cst = time.FixedZone("CST", 8*3600)

func or(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func biliVideo(v any) item {
	return item{str(v, "title"), or(str(v, "short_link_v2"), ifSet(str(v, "bvid"), "https://www.bilibili.com/video/%s"))}
}

func clsItem(v any) item {
	return item{firstStr(v, "title", "brief"), or(ifSet(str(v, "id"), "https://www.cls.cn/detail/%s"), str(v, "shareurl"))}
}

func wallstreetItem(v any) item {
	return item{firstStr(v, "title", "content_short", "content_text"), str(v, "uri")}
}

func qqNews(u string, o fetch.Options) source {
	return func(ctx context.Context) ([]item, error) {
		v, err := getJSON(ctx, u, o)
		if err != nil {
			return nil, err
		}
		rows := list(v, "idlist.0.newslist")
		if len(rows) > 0 {
			rows = rows[1:]
		}
		return mapItems(rows, "", func(v any) item {
			return item{str(v, "title"), ifSet(str(v, "id"), "https://new.qq.com/rain/a/%s")}
		}), nil
	}
}

func kr36(ctx context.Context) ([]item, error) {
	o := headers("Content-Type", "application/json; charset=utf-8")
	o.Method = "POST"
	o.Body = map[string]any{"partner_id": "wap", "param": map[string]int{"siteId": 1, "platformId": 2}, "timestamp": time.Now().UnixMilli()}
	v, err := getJSON(ctx, "https://gateway.36kr.com/api/mis/nav/home/nav/rank/hot", o)
	if err != nil {
		return nil, err
	}
	return mapItems(v, "data.hotRankList", func(v any) item {
		return item{str(v, "templateMaterial.widgetTitle"), ifSet(str(v, "itemId"), "https://www.36kr.com/p/%s")}
	}), nil
}

func githubTrending(ctx context.Context) ([]item, error) {
	q := url.QueryEscape("created:>" + time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02") + " stars:>0")
	v, err := getJSON(ctx, "https://api.github.com/search/repositories?q="+q+"&sort=stars&order=desc&per_page=30",
		headers("Accept", "application/vnd.github+json", "X-GitHub-Api-Version", "2022-11-28"))
	if err != nil {
		return nil, err
	}
	return mapItems(v, "items", func(v any) item { return item{str(v, "full_name"), str(v, "html_url")} }), nil
}

func kuaishou(ctx context.Context) ([]item, error) {
	const prefix = "window.__APOLLO_STATE__="
	html, err := getText(ctx, "https://www.kuaishou.com/?isHome=1", fetch.Options{})
	if err != nil {
		return nil, err
	}
	start := strings.Index(html, prefix)
	if start < 0 {
		return nil, fmt.Errorf("快手页面结构变更")
	}
	var state map[string]any
	if json.Unmarshal([]byte(balancedObject(html[start+len(prefix):])), &state) != nil {
		return nil, fmt.Errorf("快手数据解析失败")
	}
	client, _ := state["defaultClient"].(map[string]any)
	root := client[`$ROOT_QUERY.visionHotRank({"page":"home"})`]
	if root == nil {
		root = client[`$ROOT_QUERY.visionHotRank({"page":"home","platform":"web"})`]
	}
	return mapItems(root, "items", func(v any) item {
		hot := client[str(v, "id")]
		return item{str(hot, "name"), ifSet(str(hot, "photoIds.json.0"), "https://www.kuaishou.com/short-video/%s")}
	}), nil
}

func ithome(ctx context.Context) ([]item, error) {
	html, err := getText(ctx, "https://m.ithome.com/rankm/", fetch.Options{})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []item
	for _, b := range blocks(html, re(`(?is)<div[^>]*class="[^"]*placeholder[^"]*"[^>]*data-news-id="[^"]*"[^>]*>(.*?)</a>\s*</div>`)) {
		u := absURL("https://m.ithome.com/rankm/", first(b, re(`<a[^>]+href="([^"]+)"`)))
		if m := re(`(?:html|live)/(\d+)\.htm`).FindStringSubmatch(u); m != nil && len(m[1]) > 3 {
			u = fmt.Sprintf("https://www.ithome.com/0/%s/%s.htm", m[1][:3], m[1][3:])
		}
		it := item{first(b, re(`(?is)class="[^"]*plc-title[^"]*"[^>]*>(.*?)</[^>]+>`)), u}
		if key := or(it.url, it.title); key != "" && !seen[key] {
			seen[key] = true
			out = append(out, it)
		}
	}
	return out, nil
}

func smzdm(ctx context.Context) ([]item, error) {
	v, err := getJSON(ctx, "https://post.smzdm.com/rank/json_more/?unit=1", headers("Accept", "application/json, text/javascript, */*; q=0.01",
		"Referer", "https://post.smzdm.com/hot_1/", "X-Requested-With", "XMLHttpRequest"))
	if err == nil {
		return mapItems(v, "data", func(v any) item { return item{str(v, "title"), str(v, "jump_link")} }), nil
	}
	return htmlList("https://post.smzdm.com/hot_1/", fetch.Options{}, re(`(?is)<h5[^>]*class="[^"]*z-feed-title[^"]*"[^>]*>(.*?)</h5>`), func(b string) item {
		return item{first(b, re(`(?is)<a[^>]*>(.*?)</a>`)), first(b, re(`(?i)<a[^>]+href="([^"]+)"`))}
	})(ctx)
}

func cto51(ctx context.Context) ([]item, error) {
	tok, err := getJSON(ctx, "https://api-media.51cto.com/api/token-get", fetch.Options{})
	if err != nil {
		return nil, err
	}
	token := str(tok, "data.data.token")
	ts := time.Now().UnixMilli()
	md := func(s string) string { h := md5.Sum([]byte(s)); return hex.EncodeToString(h[:]) }
	// The signature joins the sorted parameter names, as JavaScript String(array) does.
	keys := "limit_time,name_en,page,page_size,timestamp,token"
	sign := md(md("index/index/recommend") + md(keys+md(token)+fmt.Sprint(ts)))
	q := url.Values{"page": {"1"}, "page_size": {"50"}, "limit_time": {"0"}, "name_en": {""}, "timestamp": {fmt.Sprint(ts)}, "token": {token}, "sign": {sign}}
	v, err := getJSON(ctx, "https://api-media.51cto.com/index/index/recommend?"+q.Encode(), fetch.Options{})
	if err != nil {
		return nil, err
	}
	return mapItems(v, "data.data.list", func(v any) item { return item{str(v, "title"), str(v, "url")} }), nil
}

func zhihuDaily(ctx context.Context) ([]item, error) {
	o := headers("Referer", "https://daily.zhihu.com/")
	v, err := getJSON(ctx, "https://daily.zhihu.com/api/4/news/latest", o)
	if err != nil {
		return nil, err
	}
	stories := list(v, "stories")
	date := str(v, "date")
	for page := 0; len(stories) < 30 && date != "" && page < 8; page++ {
		before, err := getJSON(ctx, "https://daily.zhihu.com/api/4/news/before/"+date, o)
		if err != nil {
			break
		}
		stories = append(stories, list(before, "stories")...)
		date = str(before, "date")
	}
	seen := map[string]bool{}
	var out []item
	for _, s := range stories {
		key := or(str(s, "id"), or(str(s, "url"), str(s, "title")))
		if key == "" || seen[key] || num(s, "type") != 0 {
			continue
		}
		seen[key] = true
		out = append(out, item{str(s, "title"), str(s, "url")})
	}
	return out, nil
}

// timed sorts items newest first by a timestamp string, which every caller formats so
// that string order is time order.
type timed struct {
	item
	at string
}

func newestFirst(rows []timed) []item {
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].at > rows[j].at })
	out := make([]item, len(rows))
	for i, r := range rows {
		out[i] = r.item
	}
	return out
}

func cankaoxiaoxi(ctx context.Context) ([]item, error) {
	var rows []timed
	for _, ch := range []string{"zhongguo", "guandian", "gj"} {
		v, err := getJSON(ctx, "https://china.cankaoxiaoxi.com/json/channel/"+ch+"/list.json", fetch.Options{})
		if err != nil {
			return nil, err
		}
		for _, e := range list(v, "list") {
			rows = append(rows, timed{item{str(e, "data.title"), str(e, "data.url")}, str(e, "data.publishTime")})
		}
	}
	return newestFirst(rows), nil
}

func v2exShare(ctx context.Context) ([]item, error) {
	var rows []timed
	for _, feed := range []string{"create", "ideas", "programmer", "share"} {
		v, err := getJSON(ctx, "https://www.v2ex.com/feed/"+feed+".json", fetch.Options{})
		if err != nil {
			return nil, err
		}
		for _, e := range list(v, "items") {
			rows = append(rows, timed{item{str(e, "title"), str(e, "url")}, firstStr(e, "date_modified", "date_published")})
		}
	}
	return newestFirst(rows), nil
}

func fastbull(ctx context.Context) ([]item, error) {
	html, err := getText(ctx, "https://www.fastbull.com/cn/express-news", fetch.Options{})
	if err != nil {
		return nil, err
	}
	var out []item
	for _, m := range re(`(?is)<div[^>]*class="[^"]*news-list[^"]*"[^>]*data-date="([^"]+)".*?<div[^>]*class="[^"]*shear_box box_show[^"]*"[^>]*data-title="([^"]+)"[^>]*data-href="([^"]+)"`).FindAllStringSubmatch(html, -1) {
		title := fetch.StripHTML(m[2])
		if t := re(`【(.+)】`).FindStringSubmatch(title); t != nil {
			title = t[1]
		}
		out = append(out, item{title, absURL("https://www.fastbull.com", m[3])})
	}
	return out, nil
}

func xueqiu(ctx context.Context) ([]item, error) {
	o := fetch.Options{}
	if cookie := xueqiuCookie(ctx); cookie != "" {
		o = headers("Cookie", cookie)
	}
	v, err := getJSON(ctx, "https://stock.xueqiu.com/v5/stock/hot_stock/list.json?size=30&_type=10&type=10", o)
	if err != nil {
		return nil, err
	}
	var out []item
	for _, e := range list(v, "data.items") {
		if num(e, "ad") == 0 {
			out = append(out, item{str(e, "name"), ifSet(str(e, "code"), "https://xueqiu.com/s/%s")})
		}
	}
	return out, nil
}

func qqVideo(ctx context.Context) ([]item, error) {
	o := headers("Referer", "https://v.qq.com/")
	o.Method = "POST"
	o.Body = json.RawMessage(qqVideoBody)
	v, err := getJSON(ctx, "https://pbaccess.video.qq.com/trpc.vector_layout.page_view.PageService/getCard?video_appid=3000010&vversion_platform=2", o)
	if err != nil {
		return nil, err
	}
	return mapItems(v, "data.card.children_list.list.cards", func(v any) item {
		return item{str(v, "params.title"), ifSet(str(v, "id"), "https://v.qq.com/x/cover/%s.html")}
	}), nil
}

const qqVideoBody = `{"page_params":{"rank_channel_id":"100113","rank_name":"HotSearch","rank_page_size":"30","tab_mvl_sub_mod_id":"792ac_19e77Sub_1b2","tab_name":"热搜榜","tab_type":"hot_rank","tab_vl_data_src":"f5200deb4596bbf3","page_id":"scms_shake","page_type":"scms_shake","source_key":"","tag_id":"","tag_type":"","new_mark_label_enabled":"1"},"page_context":{"page_index":"1"},"flip_info":{"page_strategy_id":"","page_module_id":"792ac_19e77","module_strategy_id":{},"sub_module_id":"20251106065177","flip_params":{"folding_screen_show_num":"","is_mvl":"1","mvl_strategy_info":"{\"default_strategy_id\":\"06755800b45b49238582a6fa1ad0f5c5\",\"default_version\":\"3836\",\"hit_page_uuid\":\"b5080d97dc694a5fb50eb9e7c99326ac\",\"hit_tab_info\":null,\"gray_status_info\":null,\"bypass_to_un_exp_id\":\"\"}","mvl_sub_mod_id":"20251106065177","pad_post_show_num":"","pad_pro_post_show_num":"","pad_pro_small_hor_pic_display_num":"","pad_small_hor_pic_display_num":"","page_id":"scms_shake","page_num":"0","page_type":"scms_shake","post_show_num":"","shake_size":"","small_hor_pic_display_num":"","source_key":"100113","un_policy_id":"06755800b45b49238582a6fa1ad0f5c5","un_strategy_id":"06755800b45b49238582a6fa1ad0f5c5"},"relace_children_key":[]}}`

// clsQuery signs the fixed web parameters: md5(sha1(sorted query)).
func clsQuery() string {
	q := url.Values{"appName": {"CailianpressWeb"}, "os": {"web"}, "sv": {"7.7.5"}}
	return signedCLS(q.Encode(), q)
}

func clsRollQuery() string {
	q := url.Values{"app": {"CailianpressWeb"}, "last_time": {fmt.Sprint(time.Now().Unix())}, "os": {"web"}, "refresh_type": {"1"}, "rn": {"30"}, "sv": {"8.7.9"}}
	return signedCLS(q.Encode(), q)
}

func signedCLS(raw string, q url.Values) string {
	s := sha1.Sum([]byte(raw))
	m := md5.Sum([]byte(hex.EncodeToString(s[:])))
	q.Set("sign", hex.EncodeToString(m[:]))
	return q.Encode()
}

// xueqiuCookie collects the anonymous session cookies the stock API requires.
func xueqiuCookie(ctx context.Context) string {
	_, h, err := fetch.Do(ctx, "https://xueqiu.com/hq", headers("Referer", "https://xueqiu.com/"))
	if err != nil {
		return ""
	}
	var parts []string
	for _, c := range h.Values("Set-Cookie") {
		if name, _, ok := strings.Cut(c, ";"); ok || name != "" {
			parts = append(parts, strings.TrimSpace(name))
		}
	}
	return strings.Join(parts, "; ")
}
