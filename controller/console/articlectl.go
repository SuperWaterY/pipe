// Pipe - A small and beautiful blogging platform written in golang.
// Copyright (C) 2017-2019, b3log.org & hacpai.com
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package console defines console controllers.
package console

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	"github.com/b3log/pipe/log"
	"github.com/b3log/pipe/model"
	"github.com/b3log/pipe/service"
	"github.com/b3log/pipe/util"
	"github.com/fatih/structs"
	"github.com/gin-gonic/gin"
	"github.com/parnurzeal/gorequest"
)

// Logger
var logger = log.NewLogger(os.Stdout)

// MarkdownAction handles markdown text to HTML.
func MarkdownAction(c *gin.Context) {
	result := util.NewResult()
	defer c.JSON(http.StatusOK, result)

	arg := map[string]interface{}{}
	if err := c.BindJSON(&arg); nil != err {
		result.Code = -1
		result.Msg = "parses markdown request failed"

		return
	}

	mdText := arg["markdownText"].(string)
	mdResult := util.Markdown(mdText)
	result.Data = mdResult.ContentHTML
}

var uploadTokenCheckTime, uploadTokenTime int64
var uploadToken, uploadURL = "", "https://hacpai.com/upload/client"

// UploadToken gets a upload token.
func UploadToken(c *gin.Context) {
	result := util.NewResult()
	defer c.JSON(http.StatusOK, result)

	session := util.GetSession(c)

	if "" == session.UB3Key {
		result.Code = 1

		return
	}

	data := map[string]interface{}{}
	result.Data = &data
	now := time.Now().Unix()
	if 3600 >= now-uploadTokenTime {
		data["uploadToken"] = uploadToken
		data["uploadURL"] = uploadURL

		return
	}

	if 15 >= now-uploadTokenCheckTime {
		data["uploadToken"] = uploadToken
		data["uploadURL"] = uploadURL

		return
	}

	requestJSON := map[string]interface{}{
		"userName":  session.UName,
		"userB3Key": session.UB3Key,
	}

	requestResult := util.NewResult()
	_, _, errs := gorequest.New().Post(util.HacPaiURL+"/apis/upload/token").
		SendStruct(requestJSON).Set("user-agent", model.UserAgent).Timeout(10 * time.Second).EndStruct(requestResult)
	uploadTokenCheckTime = now
	if nil != errs {
		result.Code = 1
		logger.Errorf("get upload token failed: %s", errs)

		return
	}
	if 0 != requestResult.Code {
		result.Code = 1
		result.Msg = requestResult.Msg
		logger.Errorf(requestResult.Msg)

		return
	}

	resultData := requestResult.Data.(map[string]interface{})
	uploadToken = resultData["token"].(string)
	uploadURL = resultData["uploadURL"].(string)
	uploadTokenTime = now
	result.Data = requestResult.Data
}

// AddArticleAction adds a new article.
func AddArticleAction(c *gin.Context) {
	result := util.NewResult()
	defer c.JSON(http.StatusOK, result)

	arg := map[string]interface{}{}
	if err := c.BindJSON(&arg); nil != err {
		result.Code = -1
		result.Msg = "parses add article request failed"

		return
	}

	createdAt, err := dateparse.ParseAny(arg["time"].(string))
	if nil != err {
		if "" != arg["time"].(string) {
			result.Code = -1
			result.Msg = "parses article create time failed"

			return
		}

		createdAt = time.Now()
	}

	session := util.GetSession(c)

	article := &model.Article{
		Title:       arg["title"].(string),
		Abstract:    arg["abstract"].(string),
		Content:     arg["content"].(string),
		Path:        arg["path"].(string),
		Tags:        arg["tags"].(string),
		Commentable: arg["commentable"].(bool),
		Topped:      arg["topped"].(bool),
		IP:          util.GetRemoteAddr(c),
		BlogID:      session.BID,
		AuthorID:    session.UID,
	}
	article.CreatedAt = createdAt

	if !arg["syncToCommunity"].(bool) {
		article.PushedAt = article.CreatedAt
	}

	if err := service.Article.AddArticle(article); nil != err {
		result.Code = -1
		result.Msg = err.Error()
	}
}

// GetArticleAction gets an article.
func GetArticleAction(c *gin.Context) {
	result := util.NewResult()
	defer c.JSON(http.StatusOK, result)

	idArg := c.Param("id")
	id, err := strconv.ParseUint(idArg, 10, 64)
	if nil != err {
		result.Code = -1

		return
	}

	article := service.Article.ConsoleGetArticle(uint64(id))
	if nil == article {
		result.Code = -1

		return
	}

	data := structs.Map(article)
	data["time"] = article.CreatedAt.Format("2006-01-02 15:04:05")

	result.Data = data
}

// GetArticlesAction gets articles.
func GetArticlesAction(c *gin.Context) {
	result := util.NewResult()
	defer c.JSON(http.StatusOK, result)

	session := util.GetSession(c)
	articleModels, pagination := service.Article.ConsoleGetArticles(c.Query("key"), util.GetPage(c), session.BID)
	blogURLSetting := service.Setting.GetSetting(model.SettingCategoryBasic, model.SettingNameBasicBlogURL, session.BID)

	var articles []*ConsoleArticle
	for _, articleModel := range articleModels {
		var consoleTags []*ConsoleTag
		tagStrs := strings.Split(articleModel.Tags, ",")
		for _, tagStr := range tagStrs {
			consoleTag := &ConsoleTag{
				Title: tagStr,
				URL:   blogURLSetting.Value + util.PathTags + "/" + tagStr,
			}
			consoleTags = append(consoleTags, consoleTag)
		}

		authorModel := service.User.GetUser(articleModel.AuthorID)
		author := &ConsoleAuthor{
			Name:      authorModel.Name,
			URL:       blogURLSetting.Value + util.PathAuthors + "/" + authorModel.Name,
			AvatarURL: authorModel.AvatarURL,
		}

		article := &ConsoleArticle{
			ID:           articleModel.ID,
			Author:       author,
			CreatedAt:    articleModel.CreatedAt.Format("2006-01-02"),
			Title:        articleModel.Title,
			Tags:         consoleTags,
			URL:          blogURLSetting.Value + articleModel.Path,
			Topped:       articleModel.Topped,
			ViewCount:    articleModel.ViewCount,
			CommentCount: articleModel.CommentCount,
		}

		articles = append(articles, article)
	}

	data := map[string]interface{}{}
	data["articles"] = articles
	data["pagination"] = pagination
	result.Data = data
}

// RemoveArticleAction removes an article.
func RemoveArticleAction(c *gin.Context) {
	result := util.NewResult()
	defer c.JSON(http.StatusOK, result)

	idArg := c.Param("id")
	id, err := strconv.ParseUint(idArg, 10, 64)
	if nil != err {
		result.Code = -1
		result.Msg = err.Error()

		return
	}

	session := util.GetSession(c)
	blogID := session.BID

	if err := service.Article.RemoveArticle(id, blogID); nil != err {
		result.Code = -1
		result.Msg = err.Error()
	}
}

// RemoveArticlesAction removes articles.
func RemoveArticlesAction(c *gin.Context) {
	result := util.NewResult()
	defer c.JSON(http.StatusOK, result)

	arg := map[string]interface{}{}
	if err := c.BindJSON(&arg); nil != err {
		result.Code = -1
		result.Msg = "parses batch remove articles request failed"

		return
	}

	session := util.GetSession(c)
	blogID := session.BID

	ids := arg["ids"].([]interface{})
	for _, id := range ids {
		if err := service.Article.RemoveArticle(uint64(id.(float64)), blogID); nil != err {
			logger.Errorf("remove article failed: " + err.Error())
		}
	}
}

// UpdateArticleAction updates an article.
func UpdateArticleAction(c *gin.Context) {
	result := util.NewResult()
	defer c.JSON(http.StatusOK, result)

	idArg := c.Param("id")
	id, err := strconv.ParseUint(idArg, 10, 64)
	if nil != err {
		result.Code = -1
		result.Msg = err.Error()

		return
	}

	arg := map[string]interface{}{}
	if err := c.BindJSON(&arg); nil != err {
		result.Code = -1
		result.Msg = "parses update article request failed"

		return
	}

	createdAt, err := dateparse.ParseAny(arg["time"].(string))
	if nil != err {
		result.Code = -1
		result.Msg = "parses article create time failed"

		return
	}

	session := util.GetSession(c)

	article := &model.Article{
		Model:       model.Model{ID: id, CreatedAt: createdAt},
		Title:       arg["title"].(string),
		Abstract:    arg["abstract"].(string),
		Content:     arg["content"].(string),
		Path:        arg["path"].(string),
		Tags:        arg["tags"].(string),
		Commentable: arg["commentable"].(bool),
		Topped:      arg["topped"].(bool),
		IP:          util.GetRemoteAddr(c),
		BlogID:      session.BID,
		AuthorID:    session.UID,
	}

	if err := service.Article.UpdateArticle(article); nil != err {
		result.Code = -1
		result.Msg = err.Error()
	}
}

// GetArticleThumbsAction gets article thumbnails.
func GetArticleThumbsAction(c *gin.Context) {
	result := util.NewResult()
	defer c.JSON(http.StatusOK, result)

	n, _ := strconv.Atoi(c.Query("n"))
	urls := util.RandImages(n)

	// original: 1920*1080

	w, _ := strconv.Atoi(c.Query("w"))
	if w < 1 {
		w = 960
	}
	h, _ := strconv.Atoi(c.Query("h"))
	if h < 1 {
		h = 520
	}

	var styledURLs []string
	for _, url := range urls {
		styledURLs = append(styledURLs, util.ImageSize(url, w, h))
	}

	result.Data = styledURLs
}
