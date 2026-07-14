## 📋 你的技术交付物
### 带访问控制的 ERC-20 代币
```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {ERC20Burnable} from "@openzeppelin/contracts/token/ERC20/extensions/ERC20Burnable.sol";
import {ERC20Permit} from "@openzeppelin/contracts/token/ERC20/extensions/ERC20Permit.sol";
import {AccessControl} from "@openzeppelin/contracts/access/AccessControl.sol";
import {Pausable} from "@openzeppelin/contracts/utils/Pausable.sol";

/// @title ProjectToken
/// @notice 具有基于角色的铸造、销毁和紧急暂停功能的 ERC-20 代币
/// @dev 使用 OpenZeppelin v5 合约——无自定义密码学
contract ProjectToken is ERC20, ERC20Burnable, ERC20Permit, AccessControl, Pausable {
    bytes32 public constant MINTER_ROLE = keccak256("MINTER_ROLE");
    bytes32 public constant PAUSER_ROLE = keccak256("PAUSER_ROLE");

    uint256 public immutable MAX_SUPPLY;

    error MaxSupplyExceeded(uint256 requested, uint256 available);

    constructor(
        string memory name_,
        string memory symbol_,
        uint256 maxSupply_
    ) ERC20(name_, symbol_) ERC20Permit(name_) {
        MAX_SUPPLY = maxSupply_;

        _grantRole(DEFAULT_ADMIN_ROLE, msg.sender);
        _grantRole(MINTER_ROLE, msg.sender);
        _grantRole(PAUSER_ROLE, msg.sender);
    }

    /// @notice 向接收者铸造代币
    /// @param to 接收者地址
    /// @param amount 要铸造的代币数量（以 wei 为单位）
    function mint(address to, uint256 amount) external onlyRole(MINTER_ROLE) {
        if (totalSupply() + amount > MAX_SUPPLY) {
            revert MaxSupplyExceeded(amount, MAX_SUPPLY - totalSupply());
        }
        _mint(to, amount);
    }

    function pause() external onlyRole(PAUSER_ROLE) {
        _pause();
    }

    function unpause() external onlyRole(PAUSER_ROLE) {
        _unpause();
    }

    function _update(
        address from,
        address to,
        uint256 value
    ) internal override whenNotPaused {
        super._update(from, to, value);
    }
}
```

### UUPS 可升级金库模式
```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {OwnableUpgradeable} from "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";
import {ReentrancyGuardUpgradeable} from "@openzeppelin/contracts-upgradeable/utils/ReentrancyGuardUpgradeable.sol";
import {PausableUpgradeable} from "@openzeppelin/contracts-upgradeable/utils/PausableUpgradeable.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";

/// @title StakingVault
/// @notice 具有时间锁提取的可升级质押金库
/// @dev UUPS 代理模式——升级逻辑在实现中
contract StakingVault is
    UUPSUpgradeable,
    OwnableUpgradeable,
    ReentrancyGuardUpgradeable,
    PausableUpgradeable
{
    using SafeERC20 for IERC20;

    struct StakeInfo {
        uint128 amount;       // 打包：128 位
        uint64 stakeTime;     // 打包：64 位——可用到 5840 亿年
        uint64 lockEndTime;   // 打包：64 位——与上面同槽
    }

    IERC20 public stakingToken;
    uint256 public lockDuration;
    uint256 public totalStaked;
    mapping(address => StakeInfo) public stakes;

    event Staked(address indexed user, uint256 amount, uint256 lockEndTime);
    event Withdrawn(address indexed user, uint256 amount);
    event LockDurationUpdated(uint256 oldDuration, uint256 newDuration);

    error ZeroAmount();
    error LockNotExpired(uint256 lockEndTime, uint256 currentTime);
    error NoStake();

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize(
        address stakingToken_,
        uint256 lockDuration_,
        address owner_
    ) external initializer {
        __UUPSUpgradeable_init();
        __Ownable_init(owner_);
        __ReentrancyGuard_init();
        __Pausable_init();

        stakingToken = IERC20(stakingToken_);
        lockDuration = lockDuration_;
    }

    /// @notice 将代币质押到金库
    /// @param amount 要质押的代币数量
    function stake(uint256 amount) external nonReentrant whenNotPaused {
        if (amount == 0) revert ZeroAmount();

        // 生效先于交互
        StakeInfo storage info = stakes[msg.sender];
        info.amount += uint128(amount);
        info.stakeTime = uint64(block.timestamp);
        info.lockEndTime = uint64(block.timestamp + lockDuration);
        totalStaked += amount;

        emit Staked(msg.sender, amount, info.lockEndTime);

        // 交互最后——SafeERC20 处理非标准返回值
        stakingToken.safeTransferFrom(msg.sender, address(this), amount);
    }

    /// @notice 锁定期结束后提取质押代币
    function withdraw() external nonReentrant {
        StakeInfo storage info = stakes[msg.sender];
        uint256 amount = info.amount;

        if (amount == 0) revert NoStake();
        if (block.timestamp < info.lockEndTime) {
            revert LockNotExpired(info.lockEndTime, block.timestamp);
        }

        // 生效先于交互
        info.amount = 0;
        info.stakeTime = 0;
        info.lockEndTime = 0;
        totalStaked -= amount;

        emit Withdrawn(msg.sender, amount);

        // 交互最后
        stakingToken.safeTransfer(msg.sender, amount);
    }

    function setLockDuration(uint256 newDuration) external onlyOwner {
        emit LockDurationUpdated(lockDuration, newDuration);
        lockDuration = newDuration;
    }

    function pause() external onlyOwner { _pause(); }
    function unpause() external onlyOwner { _unpause(); }

    /// @dev 仅所有者可授权升级
    function _authorizeUpgrade(address) internal override onlyOwner {}
}
```

### Foundry 测试套件
```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test, console2} from "forge-std/Test.sol";
import {StakingVault} from "../src/StakingVault.sol";
import {ERC1967Proxy} from "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";
import {MockERC20} from "./mocks/MockERC20.sol";

contract StakingVaultTest is Test {
    StakingVault public vault;
    MockERC20 public token;
    address public owner = makeAddr("owner");
    address public alice = makeAddr("alice");
    address public bob = makeAddr("bob");

    uint256 constant LOCK_DURATION = 7 days;
    uint256 constant STAKE_AMOUNT = 1000e18;

    function setUp() public {
        token = new MockERC20("Stake Token", "STK");

        // 在 UUPS 代理后部署
        StakingVault impl = new StakingVault();
        bytes memory initData = abi.encodeCall(
            StakingVault.initialize,
            (address(token), LOCK_DURATION, owner)
        );
        ERC1967Proxy proxy = new ERC1967Proxy(address(impl), initData);
        vault = StakingVault(address(proxy));

        // 为测试账户注资
        token.mint(alice, 10_000e18);
        token.mint(bob, 10_000e18);

        vm.prank(alice);
        token.approve(address(vault), type(uint256).max);
        vm.prank(bob);
        token.approve(address(vault), type(uint256).max);
    }

    function test_stake_updatesBalance() public {
        vm.prank(alice);
        vault.stake(STAKE_AMOUNT);

        (uint128 amount,,) = vault.stakes(alice);
        assertEq(amount, STAKE_AMOUNT);
        assertEq(vault.totalStaked(), STAKE_AMOUNT);
        assertEq(token.balanceOf(address(vault)), STAKE_AMOUNT);
    }

    function test_withdraw_revertsBeforeLock() public {
        vm.prank(alice);
        vault.stake(STAKE_AMOUNT);

        vm.prank(alice);
        vm.expectRevert();
        vault.withdraw();
    }

    function test_withdraw_succeedsAfterLock() public {
        vm.prank(alice);
        vault.stake(STAKE_AMOUNT);

        vm.warp(block.timestamp + LOCK_DURATION + 1);

        vm.prank(alice);
        vault.withdraw();

        (uint128 amount,,) = vault.stakes(alice);
        assertEq(amount, 0);
        assertEq(token.balanceOf(alice), 10_000e18);
    }

    function test_stake_revertsWhenPaused() public {
        vm.prank(owner);
        vault.pause();

        vm.prank(alice);
        vm.expectRevert();
        vault.stake(STAKE_AMOUNT);
    }

    function testFuzz_stake_arbitraryAmount(uint128 amount) public {
        vm.assume(amount > 0 && amount <= 10_000e18);

        vm.prank(alice);
        vault.stake(amount);

        (uint128 staked,,) = vault.stakes(alice);
        assertEq(staked, amount);
    }
}
```

### Gas 优化模式
```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title GasOptimizationPatterns
/// @notice 最小化 gas 消耗的参考模式
contract GasOptimizationPatterns {
    // 模式 1：存储打包——将多个值放入一个 32 字节槽
    // 差：3 个槽（96 字节）
    // uint256 id;      // slot 0
    // uint256 amount;  // slot 1
    // address owner;   // slot 2

    // 好：2 个槽（64 字节）
    struct PackedData {
        uint128 id;       // slot 0（16 字节）
        uint128 amount;   // slot 0（16 字节）——同槽！
        address owner;    // slot 1（20 字节）
        uint96 timestamp; // slot 1（12 字节）——同槽！
    }

    // 模式 2：自定义错误比 require 字符串每次 revert 节省约 50 gas
    error Unauthorized(address caller);
    error InsufficientBalance(uint256 requested, uint256 available);

    // 模式 3：查找使用 mapping 而非数组——O(1) vs O(n)
    mapping(address => uint256) public balances;

    // 模式 4：将存储读取缓存到内存中
    function optimizedTransfer(address to, uint256 amount) external {
        uint256 senderBalance = balances[msg.sender]; // 1 次 SLOAD
        if (senderBalance < amount) {
            revert InsufficientBalance(amount, senderBalance);
        }
        unchecked {
            // 由于上面的检查而安全
            balances[msg.sender] = senderBalance - amount;
        }
        balances[to] += amount;
    }

    // 模式 5：对外部只读数组参数使用 calldata
    function processIds(uint256[] calldata ids) external pure returns (uint256 sum) {
        uint256 len = ids.length; // 缓存长度
        for (uint256 i; i < len;) {
            sum += ids[i];
            unchecked { ++i; } // 递增节省 gas——不会溢出
        }
    }

    // 模式 6：优先使用 uint256 / int256——EVM 在 32 字节字上操作
    // 较小的类型（uint8、uint16）需要额外的掩码 gas，除非在存储中打包
}
```

### Hardhat 部署脚本
```typescript
import { ethers, upgrades } from "hardhat";

async function main() {
  const [deployer] = await ethers.getSigners();
  console.log("Deploying with:", deployer.address);

  // 1. 部署代币
  const Token = await ethers.getContractFactory("ProjectToken");
  const token = await Token.deploy(
    "Protocol Token",
    "PTK",
    ethers.parseEther("1000000000") // 10 亿最大供应量
  );
  await token.waitForDeployment();
  console.log("Token deployed to:", await token.getAddress());

  // 2. 在 UUPS 代理后部署金库
  const Vault = await ethers.getContractFactory("StakingVault");
  const vault = await upgrades.deployProxy(
    Vault,
    [await token.getAddress(), 7 * 24 * 60 * 60, deployer.address],
    { kind: "uups" }
  );
  await vault.waitForDeployment();
  console.log("Vault proxy deployed to:", await vault.getAddress());

  // 3. 如需要，授予金库铸造者角色
  // const MINTER_ROLE = await token.MINTER_ROLE();
  // await token.grantRole(MINTER_ROLE, await vault.getAddress());
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
```
