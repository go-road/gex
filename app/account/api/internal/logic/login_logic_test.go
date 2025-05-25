package logic_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/luxun9527/gex/app/account/api/internal/logic"
	"github.com/luxun9527/gex/app/account/api/internal/svc"
	"github.com/luxun9527/gex/app/account/api/internal/types"
	accountservice "github.com/luxun9527/gex/app/account/rpc/accountservice"
	assert "github.com/stretchr/testify/assert"
	"github.com/zeromicro/go-zero/rest"
	"google.golang.org/grpc"
)

// TestLoginLogic_Login 测试登录逻辑
// 在测试代码中Mock验证码校验逻辑，确保验证码校验通过。
// 然后，Mock账户服务的登录逻辑，确保登录成功。
// 最后，Mock认证服务的中间件逻辑，确保认证通过。
// 这种Mock实现方式在单元测试中不推荐使用，因为这样会引入额外的依赖，并且测试代码编写很耗时间，而且测试无法覆盖到真实的代码逻辑。
func TestLoginLogic_Login(t *testing.T) {
    // 创建mock验证码存储
    mockCaptchaStore := &mockCaptchaStore{verifyResult: true}	
	assert.Equal(t, true, mockCaptchaStore.verifyResult)
	assert.Equal(t, true, mockCaptchaStore.Verify("captcha_id", "captcha_value", true))
    
    // 创建服务上下文
    svcCtx := &svc.ServiceContext{
        CaptchaStore: mockCaptchaStore,
		AccountRpcClient: &mockAccountService{}, // 模拟账户服务
        Auth:             (&mockAuthMiddleware{}).Middleware(), // 模拟认证服务
        // ... 其他依赖 ...
    }
    
    // 测试登录逻辑
    logic := logic.NewLoginLogic(context.Background(), svcCtx)
    resp, err := logic.Login(&types.LoginReq{
        Username: "test",
        Password: "test123",
    })
    
	fmt.Println("resp:", resp)
    // 断言结果
    assert.Nil(t, err)
    assert.NotNil(t, resp)
}


/**
mockCaptchaStore 结构体需要实现 base64Captcha.Store 接口的所有方法，包括 Verify 和 Get。
Get 方法用于根据验证码 ID 获取验证码值，这里返回一个模拟值 "mock_captcha_value"，可以根据测试需求调整返回值。
*/
type mockCaptchaStore struct {
    verifyResult bool
}

func (m *mockCaptchaStore) Verify(id, answer string, clear bool) bool {
    return m.verifyResult
}

func (m *mockCaptchaStore) Get(id string, clear bool) string {
    // 这里可以根据需要返回一个模拟的验证码值
    return "mock_captcha_value"
}

func (m *mockCaptchaStore) Set(id string, value string) error {
	// 这里可以根据需要实现设置验证码的逻辑
	return nil
}

type mockAccountService struct{}

func (m *mockAccountService) GetUserByUsername(username string) (*types.UserInfo, error) {
    // 模拟返回一个用户
    return &types.UserInfo{
        Username: "test1",
    }, nil
}


func (m *mockAccountService) GetUserAssetByCoin(ctx context.Context, in *accountservice.GetUserAssetReq, opts ...grpc.CallOption) (*accountservice.GetUserAssetResp, error){
	// 模拟返回用户资产
	return &accountservice.GetUserAssetResp{

	}, nil
}
// 获取用户所有币种资产。
func (m *mockAccountService) GetUserAssetList(ctx context.Context, in *accountservice.GetUserAssetListReq, opts ...grpc.CallOption) (*accountservice.GetUserAssetListResp, error){
	// 模拟返回用户资产列表
	return &accountservice.GetUserAssetListResp{

	}, nil
}
// 冻结用户资产。
func (m *mockAccountService) FreezeUserAsset(ctx context.Context, in *accountservice.FreezeUserAssetReq, opts ...grpc.CallOption) (*accountservice.Empty, error) {	
	// 模拟冻结用户资产
	return &accountservice.Empty{}, nil	
}
// 解冻用户资产
func (m *mockAccountService) UnFreezeUserAsset(ctx context.Context, in *accountservice.FreezeUserAssetReq, opts ...grpc.CallOption) (*accountservice.Empty, error) {
	// 模拟解冻用户资产
	return &accountservice.Empty{}, nil
}	
// 扣减用户资产
func (m *mockAccountService) DeductUserAsset(ctx context.Context, in *accountservice.DeductUserAssetReq, opts ...grpc.CallOption) (*accountservice.Empty, error) {
	// 模拟扣减用户资产
	return &accountservice.Empty{}, nil
}	
// 增加用户资产
func (m *mockAccountService) AddUserAsset(ctx context.Context, in *accountservice.AddUserAssetReq, opts ...grpc.CallOption) (*accountservice.Empty, error) {
	// 模拟增加用户资产
	return &accountservice.Empty{}, nil
}	
// 注册
func (m *mockAccountService) Register(ctx context.Context, in *accountservice.RegisterReq, opts ...grpc.CallOption) (*accountservice.RegisterResp, error) {
	// 模拟注册
	return &accountservice.RegisterResp{
		Uid:      1,
		Username: in.Username,
	}, nil
}
// 登录
func (m *mockAccountService) Login(ctx context.Context, in *accountservice.LoginReq, opts ...grpc.CallOption) (*accountservice.LoginResp, error) {
 return &accountservice.LoginResp{
		Uid:      1,
		Username: "Mock Username",
		Token:    "Mock Token",
		ExpireTime: time.Now().Add(time.Hour * 24).Unix(),
	}, nil	
}
// 登出
func (m *mockAccountService) LoginOut(ctx context.Context, in *accountservice.LoginOutReq, opts ...grpc.CallOption) (*accountservice.Empty, error) {
	// 模拟登出
	return &accountservice.Empty{}, nil
}	

// 验证token是否有效。
func (m *mockAccountService) ValidateToken(ctx context.Context, in *accountservice.ValidateTokenReq, opts ...grpc.CallOption) (*accountservice.ValidateTokenResp, error) {
	return &accountservice.ValidateTokenResp{
			Uid:      1,
			Username: "test",
		}, nil	
}

type mockAuthMiddleware struct{}

func (m *mockAuthMiddleware) Middleware() rest.Middleware {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            // 这里可以添加测试需要的 mock 逻辑
            next(w, r)
        }
    }
}



