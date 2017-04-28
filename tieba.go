package tgod

import (
	"github.com/Sirupsen/logrus"
	"github.com/go-tgod/tgod/talpa"
	"github.com/go-tgod/tgod/tieba"
	gen "gopkg.in/h2non/gentleman.v2"
)

func NewTiebaSpider(forum string) *TiebaSpider {
	spider := new(TiebaSpider)
	spider.forum = forum
	spider.logger = Logger.WithField("TiebaSpider", forum)
	return spider
}

type TiebaSpider struct {
	forum  string
	logger *logrus.Entry
}

// 初始请求, 获取置顶帖吧最新(第一页)帖子列表
func (t *TiebaSpider) StartRequests() []*gen.Request {
	req := tieba.ThreadListRequest(t.forum, 1, 10, false)
	req.Context.Set("CallBack", t.ParseThreadList)
	return []*gen.Request{req}
}

// 解析帖子列表, 生成每个帖子回复列表第一页请求用于得到回帖页数进行下一步请求
func (t *TiebaSpider) ParseThreadList(res *gen.Response, helper talpa.Helper) {
	// todo: 处理帖子列表数据
	// 解析 json, 出错时会直接 panic 而不是返回 errCode
	entry := t.logger.WithField("CallBack", "ParseThreadList")
	tlr := new(tieba.ThreadListResponse)
	if err := res.JSON(tlr); err != nil {
		t.logger.Panicln(err)
	}
	if err := tlr.CheckStatus(); err != nil {
		entry.WithField("Error", err).Warnln("获取第一页帖子失败")
		return
	}

	//helper.PutJob(ForumUpsert(tlr.Forum))
	//helper.PutJob(ThreadUpsert(tlr.ThreadList...))
	if len(tlr.UserList) > 0 {
		helper.PutJob(UserUpsert(tlr.UserList...))
	}
	// todo: 当帖子最后更新时间小于上一次最新帖子更新时间则跳过
	reqs := make([]*gen.Request, len(tlr.ThreadList))
	entry.WithFields(logrus.Fields{"NumRequest": len(reqs)}).Debugln()
	for i, thread := range tlr.ThreadList {
		req := tieba.PostListRequest(thread.ID, 1, 5, true)
		//req := tieba.PostListRequest(thread.ID, 1, tieba.MaxPostNum, true)
		req.Context.Set("CallBack", t.ParsePostListPage)
		reqs[i] = req
	}
	helper.PutRequest(reqs...)
}

func (t *TiebaSpider) handlePostList(entry *logrus.Entry, res *gen.Response, helper talpa.Helper) tieba.PostListResponse {
	plr := new(tieba.PostListResponse)
	if err := res.JSON(plr); err != nil {
		panic(err)
	}
	if err := plr.CheckStatus(); err != nil {
		entry.WithField("Error", err).Warnln("获取帖子第一页楼层失败")
		return *plr
	}
	entry.WithFields(logrus.Fields{
		"ThreadTitle": plr.Thread.Title,
		"ThreadID":    plr.Thread.ID,
		// fixme: TotalPage与第一页不一致
		"TotalPage":   plr.Page.TotalPage,
		"Page":        plr.Page.CurrentPage,
		"NumPostList": len(plr.PostList),
	}).Debugln()
	helper.PutJob(ForumUpsert(plr.Forum))
	if len(plr.UserList) > 0 {
		helper.PutJob(UserUpsert(plr.UserList...))
	}
	postList := make([]tieba.Post, len(plr.PostList))
	subpostList := make([]tieba.SubPost, 0, len(plr.PostList))
	for i, p := range plr.PostList {
		postList[i] = p.Post
		subpostList = append(subpostList, p.SubPostList...)
	}
	helper.PutJob(PostUpsert(postList...))
	if len(subpostList) > 0 {
		helper.PutJob(SubPostUpsert(subpostList...))
	}
	return *plr
}

// 解析第一页回帖, 生成后序的请求
func (t *TiebaSpider) ParsePostListPage(res *gen.Response, helper talpa.Helper) {
	// todo: 处理第一页数据
	entry := t.logger.WithField("CallBack", "ParsePostListPage")
	plr := t.handlePostList(entry, res, helper)
	// 第一页已经得到了
	reqNum := plr.Page.TotalPage - 1
	reqs := make([]*gen.Request, reqNum)
	for i := 2; i <= plr.Page.TotalPage; i++ {
		req := tieba.PostListRequest(plr.Thread.ID, i, tieba.MaxPostNum, true)
		req.Context.Set("CallBack", t.ParsePostList)
		reqs[i-2] = req
	}
	helper.PutRequest(reqs...)
}

// 解析后续回帖
func (t *TiebaSpider) ParsePostList(res *gen.Response, helper talpa.Helper) {
	entry := t.logger.WithField("CallBack", "ParsePostList")
	// todo: 处理后续回帖数据
	t.handlePostList(entry, res, helper)
}
