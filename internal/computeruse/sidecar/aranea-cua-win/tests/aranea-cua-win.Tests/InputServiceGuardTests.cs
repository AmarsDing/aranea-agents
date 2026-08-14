using Aranea.Cua.Win;
using Xunit;

namespace Aranea.Cua.Win.Tests;

/// <summary>键盘注入前台窗口守卫（无桌面依赖）</summary>
public class InputServiceGuardTests
{
    [Fact]
    public void HasForegroundWindow_Zero_IsMissing()
    {
        Assert.False(InputService.HasForegroundWindow(IntPtr.Zero));
    }

    [Fact]
    public void HasForegroundWindow_NonZero_IsPresent()
    {
        Assert.True(InputService.HasForegroundWindow(new IntPtr(1)));
    }
}
