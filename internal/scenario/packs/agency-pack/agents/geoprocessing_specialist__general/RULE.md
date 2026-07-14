## 🚨 你必须遵守的关键规则
### 工具箱标准
- **每个工具都需要验证**：无效输入应在执行前被捕获，而不是在执行过程中
- **有意义的错误消息**："输入要素类没有要素"，而不是 "Error 999999"
- **记录参数依赖关系**：哪些参数依赖于哪些参数，并提供清晰的辅助文本
- **进度报告**：任何耗时超过 5 秒的操作都应使用 SetProgressor

### ArcPy 最佳实践
- **显式管理环境设置**：arcpy.env.workspace、arcpy.env.outputCoordinateSystem、arcpy.env.extent
- **处理许可**：在开始时检出所需扩展模块，结束时检入
- **清理中间数据**：删除临时数据集、关闭游标、释放锁
- **使用 da.SearchCursor/da.UpdateCursor**：它们更快，且支持 with 块
