using Aranea.Cua.Win;
using Xunit;

namespace Aranea.Cua.Win.Tests;

/// <summary>ref 解析与代际校验逻辑测试</summary>
public class RefParserTests
{
    [Theory]
    [InlineData("g1.e42", 1, 42)]
    [InlineData("g0.e0", 0, 0)]
    [InlineData("g123.e7", 123, 7)]
    public void TryParse_ValidRef_ReturnsGenerationAndIndex(string text, int gen, int idx)
    {
        Assert.True(RefParser.TryParse(text, out var g, out var i));
        Assert.Equal(gen, g);
        Assert.Equal(idx, i);
    }

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("e42")]
    [InlineData("g1")]
    [InlineData("g1e42")]
    [InlineData("g1.42")]
    [InlineData("g.e1")]
    [InlineData("g1.e")]
    [InlineData("g-1.e2")]
    [InlineData("g1.e-2")]
    [InlineData("g1.e2.extra")]
    [InlineData("x1.e2")]
    public void TryParse_InvalidRef_ReturnsFalse(string? text)
    {
        Assert.False(RefParser.TryParse(text, out _, out _));
    }

    [Fact]
    public void Format_RoundTrips()
    {
        var text = RefParser.Format(12, 42);
        Assert.Equal("g12.e42", text);
        Assert.True(RefParser.TryParse(text, out var g, out var i));
        Assert.Equal(12, g);
        Assert.Equal(42, i);
    }

    [Fact]
    public void CrossGeneration_IsDetectable()
    {
        // 代际校验：ref 代 != 当前代 → 视为过期（对应 -32001）
        Assert.True(RefParser.TryParse("g3.e9", out var refGen, out _));
        var currentGeneration = 4;
        Assert.NotEqual(currentGeneration, refGen);
    }
}
