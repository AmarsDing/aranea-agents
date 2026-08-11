using System.Text.Json;
using Aranea.Cua.Win;
using Xunit;

namespace Aranea.Cua.Win.Tests;

/// <summary>JSON-RPC 帧解析与错误包装测试（纯逻辑，无桌面依赖）</summary>
public class JsonRpcTests
{
    [Fact]
    public void Parse_ValidRequest_ExtractsIdMethodParams()
    {
        var ok = JsonRpc.TryParseRequest("{\"jsonrpc\":\"2.0\",\"id\":7,\"method\":\"device.ping\",\"params\":{\"a\":1}}", out var req, out var err);
        Assert.True(ok);
        Assert.Null(err);
        using (req!)
        {
            Assert.Equal("7", req.IdRaw);
            Assert.Equal("device.ping", req.Method);
            Assert.True(req.Params.HasValue);
            Assert.Equal(1, req.Params.Value.GetProperty("a").GetInt32());
        }
    }

    [Fact]
    public void Parse_NoParams_ParamsIsNull()
    {
        var ok = JsonRpc.TryParseRequest("{\"id\":1,\"method\":\"device.ping\"}", out var req, out var err);
        Assert.True(ok);
        using (req!)
        {
            Assert.Null(req.Params);
        }
    }

    [Fact]
    public void Parse_InvalidJson_ReturnsParseError32700()
    {
        var ok = JsonRpc.TryParseRequest("{not json", out var req, out var err);
        Assert.False(ok);
        Assert.Null(req);
        Assert.NotNull(err);
        using var doc = JsonDocument.Parse(err!);
        Assert.Equal(JsonRpc.ParseError, doc.RootElement.GetProperty("error").GetProperty("code").GetInt32());
        Assert.Equal("null", doc.RootElement.GetProperty("id").GetRawText());
    }

    [Fact]
    public void Parse_MissingMethod_ReturnsMethodNotFound32601_AndEchoesId()
    {
        var ok = JsonRpc.TryParseRequest("{\"id\":42,\"params\":{}}", out _, out var err);
        Assert.False(ok);
        using var doc = JsonDocument.Parse(err!);
        Assert.Equal(JsonRpc.MethodNotFound, doc.RootElement.GetProperty("error").GetProperty("code").GetInt32());
        Assert.Equal(42, doc.RootElement.GetProperty("id").GetInt32());
    }

    [Fact]
    public void Parse_NonObjectFrame_ReturnsParseError()
    {
        var ok = JsonRpc.TryParseRequest("[1,2,3]", out _, out var err);
        Assert.False(ok);
        using var doc = JsonDocument.Parse(err!);
        Assert.Equal(JsonRpc.ParseError, doc.RootElement.GetProperty("error").GetProperty("code").GetInt32());
    }

    [Fact]
    public void Parse_StringId_EchoesAsString()
    {
        var ok = JsonRpc.TryParseRequest("{\"id\":\"abc-1\",\"method\":\"device.ping\"}", out var req, out _);
        Assert.True(ok);
        using (req!)
        {
            Assert.Equal("\"abc-1\"", req.IdRaw);
        }
    }

    [Fact]
    public void ResultResponse_WrapsCamelCaseResult()
    {
        var frame = JsonRpc.ResultResponse("3", new ScreenshotResultDto { PngBase64 = "AA==", Width = 2, Height = 1, ScaleFactor = 1.5 });
        using var doc = JsonDocument.Parse(frame);
        var root = doc.RootElement;
        Assert.Equal("2.0", root.GetProperty("jsonrpc").GetString());
        Assert.Equal(3, root.GetProperty("id").GetInt32());
        var result = root.GetProperty("result");
        Assert.Equal("AA==", result.GetProperty("pngBase64").GetString());
        Assert.Equal(1.5, result.GetProperty("scaleFactor").GetDouble());
    }

    [Fact]
    public void ErrorResponse_EscapesMessageAndCarriesCode()
    {
        var frame = JsonRpc.ErrorResponse("9", JsonRpc.ElementNotFound, "元素\"x\"未找到");
        using var doc = JsonDocument.Parse(frame);
        var error = doc.RootElement.GetProperty("error");
        Assert.Equal(-32001, error.GetProperty("code").GetInt32());
        Assert.Equal("元素\"x\"未找到", error.GetProperty("message").GetString());
    }

    [Fact]
    public void IsKnownMethod_KnownAndUnknown()
    {
        Assert.True(JsonRpc.IsKnownMethod("perception.snapshot"));
        Assert.True(JsonRpc.IsKnownMethod("action.invoke"));
        Assert.False(JsonRpc.IsKnownMethod("action.explode"));
    }
}
