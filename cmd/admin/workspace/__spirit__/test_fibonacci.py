#!/usr/bin/env python3
"""斐波那契数列函数的测试用例。"""

import unittest
from fibonacci import fibonacci


class TestFibonacci(unittest.TestCase):
    """3 个测试用例验证斐波那契函数正确性。"""

    def test_base_cases(self):
        """测试边界值：第0项=0，第1项=1"""
        self.assertEqual(fibonacci(0), 0)
        self.assertEqual(fibonacci(1), 1)

    def test_normal_values(self):
        """测试正常值：第10项=55，第20项=6765"""
        self.assertEqual(fibonacci(10), 55)
        self.assertEqual(fibonacci(20), 6765)

    def test_negative_input(self):
        """测试负数输入应抛出 ValueError"""
        with self.assertRaises(ValueError):
            fibonacci(-1)


if __name__ == "__main__":
    unittest.main()
