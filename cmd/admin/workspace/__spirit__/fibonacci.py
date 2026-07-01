#!/usr/bin/env python3
"""斐波那契数列函数实现。"""


def fibonacci(n: int) -> int:
    """返回第 n 项斐波那契数。

    F(0) = 0, F(1) = 1, F(n) = F(n-1) + F(n-2)

    参数:
        n: 非负整数索引

    返回:
        第 n 项斐波那契数

    异常:
        ValueError: 当 n 为负数时
    """
    if n < 0:
        raise ValueError("n 必须为非负整数")
    if n <= 1:
        return n

    a, b = 0, 1
    for _ in range(2, n + 1):
        a, b = b, a + b
    return b
