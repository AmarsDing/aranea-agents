using Aranea.Cua.Win;
using Xunit;

namespace Aranea.Cua.Win.Tests;

/// <summary>树遍历器深度/数量截断与过滤逻辑测试（合成树，无桌面依赖）</summary>
public class TreeWalkerTests
{
    private sealed class Node
    {
        public string Name = "";
        public List<Node> Children = new();
    }

    private static List<Node> Kids(Node n) => n.Children;

    /// <summary>构造 depth 层、每层 width 个孩子的满树</summary>
    private static Node BuildTree(int depth, int width, string prefix = "n")
    {
        var root = new Node { Name = prefix };
        var current = new List<Node> { root };
        for (var d = 0; d < depth; d++)
        {
            var next = new List<Node>();
            foreach (var p in current)
            {
                for (var i = 0; i < width; i++)
                {
                    var c = new Node { Name = $"{prefix}{d}{i}" };
                    p.Children.Add(c);
                    next.Add(c);
                }
            }
            current = next;
        }
        return root;
    }

    [Fact]
    public void Collect_PreOrderDfs_RootFirst()
    {
        var root = BuildTree(1, 2);
        var items = TreeWalker.Collect<Node, string>(root, Kids, n => n.Name, 12, 100);
        Assert.Equal("n", items[0]);
        Assert.Equal(3, items.Count);
    }

    [Fact]
    public void Collect_DepthLimit_StopsDescending()
    {
        var root = BuildTree(5, 1); // 链：深度 0..5 共 6 个节点
        var items = TreeWalker.Collect<Node, string>(root, Kids, n => n.Name, maxDepth: 2, maxElements: 100);
        Assert.Equal(3, items.Count); // 深度 0,1,2
    }

    [Fact]
    public void Collect_MaxElements_Truncates()
    {
        var root = BuildTree(3, 3); // 1+3+9+27=40 节点
        var items = TreeWalker.Collect<Node, string>(root, Kids, n => n.Name, maxDepth: 12, maxElements: 5);
        Assert.Equal(5, items.Count);
    }

    [Fact]
    public void Collect_SkippedItems_DontConsumeBudget()
    {
        var root = BuildTree(1, 4); // 根 + 4 子
        // 跳过名字含 "00" 的节点：maxElements 只计入选中的
        var items = TreeWalker.Collect<Node, string>(root, Kids,
            n => n.Name.Contains("00") ? null : n.Name, maxDepth: 12, maxElements: 3);
        Assert.Equal(3, items.Count);
        Assert.DoesNotContain(items, s => s.Contains("00"));
    }

    [Fact]
    public void Collect_ZeroMaxElements_ReturnsEmpty()
    {
        var root = BuildTree(1, 2);
        var items = TreeWalker.Collect<Node, string>(root, Kids, n => n.Name, 12, 0);
        Assert.Empty(items);
    }

    [Fact]
    public void Collect_ChildrenThrowing_SkipsSubtree()
    {
        var root = new Node { Name = "root" };
        var bad = new Node { Name = "bad" };
        var good = new Node { Name = "good" };
        root.Children.Add(bad);
        root.Children.Add(good);
        var items = TreeWalker.Collect<Node, string>(root,
            n => n == bad ? throw new InvalidOperationException("com dead") : n.Children,
            n => n.Name, 12, 100);
        Assert.Equal(new[] { "root", "bad", "good" }, items);
    }
}
