package logic

import (
	"context"
	"github.com/luxun9527/gex/app/account/rpc/accountservice"
	"github.com/luxun9527/gex/common/errs"
	logger "github.com/luxun9527/zlog"

	"github.com/luxun9527/gex/app/account/api/internal/svc"
	"github.com/luxun9527/gex/app/account/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LoginLogic) Login(req *types.LoginReq) (resp *types.LoginResp, err error) {
	// 如果是本地环境，跳过验证码校验
    if l.svcCtx.Config.Env == "local" {
        logx.Info("本地环境，跳过验证码校验")
    } else {
		// 验证验证码
		logx.Debugf("captcha id: %s, captcha: %s", req.CaptchaId, req.Captcha)
		if !l.svcCtx.CaptchaStore.Verify(req.CaptchaId, req.Captcha, true) {
			logx.Error("验证码校验失败")
			return nil, errs.CaptchaValidateFailed
		}
    }
	
	loginResp, err := l.svcCtx.AccountRpcClient.Login(l.ctx, &accountservice.LoginReq{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		logx.Errorw("call account login failed", logger.ErrorField(err))
		return nil, err
	}
	resp = &types.LoginResp{
		Uid:        loginResp.Uid,
		Username:   loginResp.Username,
		Token:      loginResp.Token,
		ExpireTime: loginResp.ExpireTime,
	}
	return
}


// TODO 增加风控检查
func (l *LoginLogic) checkRisk(ip string) error {
    // 实现IP频率限制、设备指纹校验等
    // 使用redis实现滑动窗口计数
	return nil
}