#!/usr/bin/env python3
"""运行斐波那契测试。"""
import sys
import unittest

sys.path.insert(0, r'F:\aranea-agents\workspace\__spirit__')

from test_fibonacci import TestFibonacci

if __name__ == "__main__":
    suite = unittest.TestLoader().loadTestsFromTestCase(TestFibonacci)
    runner = unittest.TextTestRunner(verbosity=2)
    result = runner.run(suite)
    sys.exit(0 if result.wasSuccessful() else 1)
