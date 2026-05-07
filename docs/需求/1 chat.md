
# 对话框
显示与agent和team的对话内容和对话方式，对话历史等
## 布局和详情
### 左侧agent和team列表，列表内分组显示agent和team
  1. 宽度120px 高度：100%；agent和team分组显示。
  2. 默认agent显示在agent组的最上方，可拖拽对agent进行上下调序
  3. 默认team显示在team组最上方，可拖拽对team组进行上下调序
  4. 默认agent和team无法拖拽调序，始终在最上方
  5. 顶部是按名称搜索agent和team的搜索框，带输入提示和输入过滤
  6. agent和team条目左边显示工作状态和名称，右边显示工具按钮：设置和删除
  7. 设置按钮：弹框显示agent和team的设置界面
  8. 删除按钮：弹出删除确认对话框，正在工作无法删除，删除时需填写名称后才能删除
  9. 选中agent和team时，背景高亮显示，右侧session历史记录栏显示历史session聊天记录，中间对话框显示最近session内容
  10. 首次进入默认选中默认的agent进行显示
  11. 列表右侧中间显示折叠按钮，点击折叠和现实左侧列表，带动画
### 右侧session历史聊天记录栏，列表显示
  1. 宽度120px，高度：100% 
  2. 每条右侧显示session名称，下角标显示session时间，左侧圆环显示session的上下文额度比和删除按钮
  3. 底部功能按钮，左侧新建session和右侧一键删除历史session
  4. 列表左侧中间显示折叠按钮，点击折叠和现实session，带动画
### 中间：对话区域
  1. 底部对话输入区域，初始高度100px，宽度：100%，使用autogrow属性；
    ```javascript
    <q-input
      v-model="text"
      filled
      autogrow
    />
    ```
  2. q-input输入框，输入框高度适应输入内容的量，最高高度400px，高于400px时出现滚动条。
  3. q-input框底部是工具条，固定高40px，宽度100%。
    - 左侧下拉框显示
          对话模式： 控件<q-select /> , 从后台拉去，数据库存储字段
          模型提供商：控件： <q-select />，从后台拉去，数据库存储字段
          上下文使用量(<q-circular-progress>)，从后台拉去，数据库存储字段
    - 右侧显示文件导入按钮，语音输入按钮和发送按钮。
  4. 有文件导入时，q-input框内部的上方显示30*30px的方框，显示导入文件进度，名称，鼠标移动到方框时，方框右上角显示关闭按钮。
  5. 剩余空间是对话内容框，控件q-chat-message，显示头像，时间，对话内容。
  示例:  https://www.quasar-cn.cn/vue-components/chat#example---
  ```javascript
  <template>
  <div class="q-pa-md row justify-center">
    <div style="width: 100%; max-width: 400px">
      <q-chat-message
        :text="['Have you seen Quasar?']"
        sent
        text-color="white"
        bg-color="primary"
      >
        <template v-slot:name>me</template>
        <template v-slot:stamp>7 minutes ago</template>
        <template v-slot:avatar>
          <img
            class="q-message-avatar q-message-avatar--sent"
            src="https://cdn.quasar.dev/img/avatar4.jpg"
          >
        </template>
      </q-chat-message>

      <q-chat-message
        bg-color="amber"
      >
        <template v-slot:name>Mary</template>
        <template v-slot:avatar>
          <img
            class="q-message-avatar q-message-avatar--received"
            src="https://cdn.quasar.dev/img/avatar2.jpg"
          >
        </template>

        <div>
          Already building an app with it...
          <img src="https://cdn.quasar.dev/img/discord-qeart.png" class="my-emoji">
        </div>

        <q-spinner-dots size="2rem" />
      </q-chat-message>
    </div>
  </div>
</template>

<style lang="sass">
.my-emoji
  vertical-align: middle
  height: 2em
  width: 2em
</style>
  ```

## 近期交互需求整理

1. 聊天记录在黑夜模式下必须保证正文、代码块、工具结果、时间戳等文本可读，避免低对比度文字被深色背景淹没。
2. Agent / Team 标签栏在黑夜模式下需要使用明确的选中态、文字色和图标色，确保选中项、状态标签、删除/设置按钮都清晰可见。
3. Session 需要标题：首次对话后由模型总结标题，或在模型未返回时由程序从用户首条消息自动生成短标题；标题展示在 session 列表和聊天顶部，交互参考 Cursor。
4. 输入框键盘行为：按 `Enter` 直接发送，按 `Shift + Enter` 换行。
5. 同一个 session 内允许切换模型，后续发送使用当前选择的模型，并在顶部显示当前模型。
6. 模型回复或工具执行过程中，发送按钮切换为执行中的方块停止图标；用户点击可暂停/停止当前执行。
7. 执行过程中再次发送消息时，先进入“待执行”队列，在聊天面板中可见；待执行消息尚未真正执行前，用户可以取消或编辑，执行完成后再按顺序发送。
