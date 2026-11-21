<template>
  <!-- 工作台 › 邮件：三栏（视图与账户 · 列表 · 阅读），设计 docs/aranea-agents/design/2026-11-17-三大模块重设计/09-邮件.md §3 -->
  <q-page class="app-standard-page ml-page" :class="{ 'ml-page--reading': !!selectedId }">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="sync"
        :aria-label="t('mailPage.fetch')"
        :loading="fetching || refreshing"
        @click="fetchNow()"
      >
        <q-tooltip>{{ t('mailPage.fetchHint') }}</q-tooltip>
      </q-btn>
    </Teleport>

    <AppStatePanel v-if="forbidden" class="ml-forbidden" variant="forbidden" />

    <template v-else>
      <!-- ============ 左：写信 · 收信 / 视图 / 账户 ============ -->
      <nav class="ml-panel ml-nav" :aria-label="t('mailPage.navAria')">
        <div class="ml-nav__top">
          <q-btn
            unelevated
            no-caps
            color="primary"
            icon="edit"
            class="ml-nav__compose"
            :label="t('mailPage.compose')"
            @click="startCompose('new')"
          />
          <q-btn
            outline
            no-caps
            icon="sync"
            class="ml-nav__fetch"
            :label="t('mailPage.fetch')"
            :loading="fetching"
            @click="fetchNow()"
          >
            <q-tooltip>{{ t('mailPage.fetchHint') }}</q-tooltip>
          </q-btn>
        </div>
        <button
          v-for="v in VIEWS"
          :key="v"
          type="button"
          class="ml-nv"
          :class="{ 'ml-nv--on': view === v && !accountFilter }"
          :aria-current="view === v && !accountFilter ? 'page' : undefined"
          @click="setView(v)"
        >
          <q-icon :name="VIEW_ICON[v]" size="18px" aria-hidden="true" />
          <span>{{ t(`mailPage.view.${v}`) }}</span>
          <span v-if="viewCount(v)" class="ml-nv__n ml-num">{{ viewCount(v) }}</span>
        </button>

        <div class="ml-sect">{{ t('mailPage.accounts') }}</div>
        <div v-for="a in accounts" :key="a.id" class="ml-acct">
          <button
            type="button"
            class="ml-nv"
            :class="{ 'ml-nv--on': accountFilter === a.id }"
            :title="a.email || a.username"
            @click="setAccount(a.id)"
          >
            <span class="ml-dot" :class="`ml-dot--${dotOf(a)}`" aria-hidden="true" />
            <span class="ml-nv__name">{{ a.name || a.email || a.username }}</span>
            <span v-if="a.status === 'auth_failed' || a.status === 'unreachable'" class="ml-nv__n ml-nv__n--bad">
              {{ t(`mailPage.status.${a.status}`) }}
            </span>
            <span v-else-if="summary?.accounts[a.id]" class="ml-nv__n ml-num">{{ summary?.accounts[a.id] }}</span>
          </button>
          <q-btn
            flat
            dense
            round
            size="sm"
            icon="more_horiz"
            class="ml-acct__more"
            :aria-label="t('mailPage.accountMenu', { name: a.name })"
          >
            <q-menu anchor="bottom right" self="top right">
              <q-list dense>
                <q-item v-close-popup clickable @click="fetchNow([a])">
                  <q-item-section>{{ t('mailPage.syncNow') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable @click="openSettings(a.id)">
                  <q-item-section>{{ t('mailPage.settings.accountEntry') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable @click="testAccount(a)">
                  <q-item-section>{{ t('mailPage.testConn') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable @click="editAccount(a)">
                  <q-item-section>{{ t('mailPage.editAccount') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable class="text-negative" @click="removeAccount(a)">
                  <q-item-section>{{ t('mailPage.deleteAccount') }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </div>
        <button type="button" class="ml-nv ml-nv--add" @click="addAccount">
          <q-icon name="add" size="18px" aria-hidden="true" />
          <span>{{ t('mailPage.addAccount') }}</span>
        </button>

        <template v-if="summary?.labels.length">
          <div class="ml-sect">{{ t('mailPage.labels') }}</div>
          <button
            v-for="l in summary.labels"
            :key="l.name"
            type="button"
            class="ml-nv"
            :class="{ 'ml-nv--on': label === l.name }"
            :aria-current="label === l.name ? 'page' : undefined"
            @click="setLabel(l.name)"
          >
            <q-icon name="label_outline" size="18px" aria-hidden="true" />
            <span class="ml-nv__name">{{ l.name }}</span>
            <span class="ml-nv__n ml-num">{{ l.count }}</span>
          </button>
        </template>

        <div class="ml-sect">{{ t('mailPage.alsoCan') }}</div>
        <q-btn
          flat
          dense
          no-caps
          icon="tune"
          class="ml-nav__import"
          :label="t('mailPage.settings.title')"
          @click="openSettings('')"
        />
        <p class="ml-hint ml-nav__hint">{{ t('mailPage.dropHint') }}</p>
        <q-btn
          flat
          dense
          no-caps
          icon="upload_file"
          class="ml-nav__import"
          :label="t('mailPage.importEml')"
          @click="emlPicker?.click()"
        />
        <input ref="emlPicker" type="file" accept=".eml,message/rfc822" multiple class="ml-sr" @change="onEmlPicked" />
      </nav>

      <!-- ============ 中：搜索 + 列表 ============ -->
      <section
        class="ml-panel ml-list"
        :class="{ 'ml-list--drop': dragging }"
        :aria-label="t('mailPage.listAria')"
        @dragover.prevent="onDragOver"
        @dragleave="dragging = false"
        @drop.prevent="onDrop"
      >
        <div class="ml-list__head">
          <div class="ml-list__bar">
            <label class="ml-search">
              <q-icon name="search" size="18px" aria-hidden="true" />
              <span class="ml-sr">{{ t('mailPage.search') }}</span>
              <input
                ref="searchInput"
                v-model="keyword"
                type="search"
                :placeholder="t('mailPage.searchPlaceholder')"
                @keydown.esc="keyword = ''"
              />
            </label>
            <q-btn
              flat
              dense
              round
              icon="edit"
              class="ml-list__compose"
              :aria-label="t('mailPage.compose')"
              @click="startCompose('new')"
            />
          </div>
          <div class="ml-list__bar" :class="{ 'ml-list__bar--bulk': selected.size }">
            <q-checkbox
              dense
              size="sm"
              class="ml-list__all"
              :model-value="allState"
              :disable="!rows.length"
              :aria-label="t('mailPage.selectAll')"
              @update:model-value="toggleAll"
            >
              <q-tooltip>{{ t('mailPage.selectAll') }}</q-tooltip>
            </q-checkbox>
            <!-- 选中了信：这一行换成批量操作 -->
            <template v-if="selected.size">
              <span class="ml-hint ml-num ml-list__seln" aria-live="polite">{{
                t('mailPage.selectedN', { n: selected.size })
              }}</span>
              <span class="ml-sp" />
              <q-btn
                v-for="b in bulkButtons"
                :key="b.key"
                flat
                dense
                round
                size="sm"
                :icon="b.icon"
                :color="b.danger ? 'negative' : undefined"
                :aria-label="b.label"
                :disable="acting"
                @click="bulk(b.key)"
              >
                <q-tooltip>{{ b.label }}</q-tooltip>
              </q-btn>
              <q-btn
                v-if="view !== 'trash'"
                flat
                dense
                round
                size="sm"
                icon="label_outline"
                :aria-label="t('mailPage.addLabel')"
              >
                <q-tooltip>{{ t('mailPage.addLabel') }}</q-tooltip>
                <q-menu anchor="bottom right" self="top right">
                  <q-list dense>
                    <q-item
                      v-for="l in summary?.labels ?? []"
                      :key="l.name"
                      v-close-popup
                      clickable
                      @click="applyLabel([...selected], l.name)"
                    >
                      <q-item-section avatar><q-icon name="label_outline" size="18px" /></q-item-section>
                      <q-item-section>{{ l.name }}</q-item-section>
                    </q-item>
                    <q-item v-close-popup clickable @click="newLabel([...selected])">
                      <q-item-section avatar><q-icon name="new_label" size="18px" /></q-item-section>
                      <q-item-section>{{ t('mailPage.newLabel') }}</q-item-section>
                    </q-item>
                  </q-list>
                </q-menu>
              </q-btn>
              <q-btn flat dense round size="sm" icon="close" :aria-label="t('mailPage.clearSel')" @click="clearSel">
                <q-tooltip>{{ t('mailPage.clearSel') }}</q-tooltip>
              </q-btn>
            </template>
            <template v-else>
              <q-select
                v-model="narrowView"
                class="ml-list__viewsel"
                dense
                borderless
                emit-value
                map-options
                :options="narrowOptions"
                :aria-label="t('mailPage.navAria')"
              />
              <q-btn-toggle
                v-if="view === 'inbox' || view === 'unread'"
                :model-value="view"
                dense
                no-caps
                unelevated
                toggle-color="primary"
                :options="segOptions"
                :aria-label="t('mailPage.view.inbox')"
                @update:model-value="setView"
              />
              <span v-if="label" class="ml-chip ml-chip--on">
                <q-icon name="label_outline" size="13px" aria-hidden="true" />{{ label }}
                <q-btn
                  flat
                  dense
                  round
                  size="xs"
                  icon="close"
                  :aria-label="t('mailPage.clearLabel')"
                  @click="setLabel('')"
                />
              </span>
              <span class="ml-sp" />
              <q-btn
                v-if="view !== 'sent' && view !== 'drafts' && view !== 'trash'"
                flat
                dense
                round
                size="sm"
                icon="done_all"
                :aria-label="t('mailPage.markAllRead')"
                :disable="acting"
                @click="markAllRead"
              >
                <q-tooltip>{{ t('mailPage.markAllRead') }}</q-tooltip>
              </q-btn>
              <span class="ml-hint ml-num">{{ t('mailPage.totalN', { n: total }) }}</span>
            </template>
          </div>
          <!-- 已删除 / 垃圾邮件：说明 + 清空 -->
          <div v-if="view === 'trash' || view === 'spam'" class="ml-list__bin">
            <q-icon :name="VIEW_ICON[view]" size="16px" aria-hidden="true" />
            <span class="ml-hint">{{ t(`mailPage.${view}Hint`) }}</span>
            <q-btn
              flat
              dense
              no-caps
              size="sm"
              color="negative"
              :label="t(view === 'trash' ? 'mailPage.emptyTrash' : 'mailPage.emptySpam')"
              :disable="!total || acting"
              @click="emptyView"
            />
          </div>
        </div>

        <div
          ref="listBody"
          class="ml-list__body"
          :class="{ 'ml-list__body--sel': selected.size }"
          :aria-busy="listLoading"
          @scroll="onListScroll"
        >
          <q-linear-progress
            v-if="listLoading && !rows.length"
            indeterminate
            color="primary"
            class="ml-list__progress"
          />
          <AppStatePanel
            v-if="listError"
            dense
            variant="partial"
            :description="listError"
            :action-label="t('mailPage.retry')"
            action-icon="refresh"
            @action="loadList(true)"
          />
          <div v-if="!accounts.length && !rows.length && !listLoading" class="ml-empty">
            <q-icon name="alternate_email" size="32px" aria-hidden="true" />
            <p class="ml-empty__title">{{ t('mailPage.noAccountTitle') }}</p>
            <p class="ml-hint">{{ t('mailPage.noAccountDesc') }}</p>
            <q-btn
              unelevated
              no-caps
              color="primary"
              icon="add"
              :label="t('mailPage.addAccount')"
              @click="addAccount"
            />
          </div>
          <div v-else-if="!rows.length && !listLoading && !listError" class="ml-empty">
            <q-icon :name="keyword ? 'search_off' : 'drafts'" size="32px" aria-hidden="true" />
            <p class="ml-empty__title">{{ keyword ? t('mailPage.noMatch') : t(`mailPage.empty.${view}`) }}</p>
          </div>

          <template v-for="g in groups" :key="g.key">
            <div class="ml-group">{{ t(`mailPage.group.${g.key}`) }}</div>
            <div
              v-for="m in g.rows"
              :key="m.id"
              class="ml-msg"
              :class="{
                'ml-msg--unread': !m.seen,
                'ml-msg--on': m.id === selectedId,
                'ml-msg--sel': selected.has(m.id),
              }"
            >
              <span class="ml-av" :class="`ml-av--${avatarTone(counterpart(m))}`" aria-hidden="true">{{
                initial(counterpart(m))
              }}</span>
              <q-checkbox
                dense
                size="sm"
                class="ml-msg__ck"
                :model-value="selected.has(m.id)"
                :aria-label="t('mailPage.selectOne', { subject: m.subject || t('mailPage.noSubject') })"
                @update:model-value="toggleSel(m.id)"
              />
              <span class="ml-msg__acts">
                <q-btn
                  v-for="b in rowButtons(m)"
                  :key="b.key"
                  flat
                  dense
                  round
                  size="sm"
                  :icon="b.icon"
                  :aria-label="b.label"
                  @click="rowAction(m, b.key)"
                >
                  <q-tooltip>{{ b.label }}</q-tooltip>
                </q-btn>
              </span>
              <button
                type="button"
                class="ml-msg__open"
                :aria-current="m.id === selectedId ? 'true' : undefined"
                @click="openMessage(m)"
              >
                <span class="ml-msg__main">
                  <span class="ml-msg__top">
                    <span class="ml-msg__from">{{ listWho(m) }}</span>
                    <q-icon
                      v-if="m.flagged"
                      name="star"
                      size="14px"
                      class="ml-star"
                      :aria-label="t('mailPage.flagged')"
                    />
                    <span class="ml-msg__time ml-num">{{ listTime(m.received_at, now) }}</span>
                  </span>
                  <span class="ml-msg__subj">{{ m.subject || t('mailPage.noSubject') }}</span>
                  <span class="ml-msg__snip">{{ m.snippet }}</span>
                  <span
                    v-if="m.has_attachments || m.task_ref || accounts.length > 1 || m.labels.length || m.send_at"
                    class="ml-msg__chips"
                  >
                    <span v-if="m.send_at" class="ml-chip ml-chip--on"
                      ><q-icon name="schedule_send" size="13px" aria-hidden="true" />{{
                        t('mailPage.scheduledChip', { at: fullTime(m.send_at) })
                      }}</span
                    >
                    <span v-for="l in m.labels" :key="l" class="ml-chip"
                      ><q-icon name="label_outline" size="13px" aria-hidden="true" />{{ l }}</span
                    >
                    <span v-if="m.has_attachments" class="ml-chip"
                      ><q-icon name="attach_file" size="13px" aria-hidden="true" />{{
                        t('mailPage.hasAttachments')
                      }}</span
                    >
                    <span v-if="m.task_ref" class="ml-chip">{{ t('mailPage.taskLinked') }}</span>
                    <span v-if="accounts.length > 1 && accountName(m.account_id)" class="ml-msg__acct">{{
                      accountName(m.account_id)
                    }}</span>
                  </span>
                </span>
              </button>
            </div>
          </template>
          <div v-if="loadingMore" class="ml-hint ml-list__more">{{ t('mailPage.loading') }}</div>
        </div>
      </section>

      <!-- ============ 右：阅读 ============ -->
      <section class="ml-panel ml-read" :aria-label="t('mailPage.readAria')">
        <div v-if="!detail" class="ml-empty ml-empty--read">
          <q-icon name="mark_email_read" size="36px" aria-hidden="true" />
          <p class="ml-hint">{{ detailLoading ? t('mailPage.loading') : t('mailPage.pickMessage') }}</p>
        </div>
        <template v-else>
          <header class="ml-read__head">
            <q-btn
              flat
              dense
              round
              icon="arrow_back"
              class="ml-read__back"
              :aria-label="t('mailPage.back')"
              @click="closeReader"
            />
            <h2 class="ml-read__subj">{{ detail.message.subject || t('mailPage.noSubject') }}</h2>
            <div class="ml-read__who">
              <span class="ml-av ml-av--lg" :class="`ml-av--${avatarTone(detail.message.sender)}`" aria-hidden="true">
                {{ initial(detail.message.sender) }}
              </span>
              <span class="ml-read__name">
                <b>{{ displayName(detail.message.sender) || t('mailPage.unknownSender') }}</b>
                <span class="ml-read__addr ml-num">{{ parseAddr(detail.message.sender).address }}</span>
              </span>
              <span class="ml-read__time ml-num">{{ fullTime(detail.message.received_at) }}</span>
              <span class="ml-read__to">
                <template v-if="detail.message.to">{{ t('mailPage.sentTo', { to: detail.message.to }) }}</template>
                <template v-if="detail.message.cc"> · {{ t('mailPage.ccTo', { cc: detail.message.cc }) }}</template>
                <template v-if="accountName(detail.message.account_id)">
                  · {{ accountName(detail.message.account_id) }}</template
                >
              </span>
            </div>
            <div v-if="detail.message.labels.length" class="ml-read__labels">
              <span v-for="l in detail.message.labels" :key="l" class="ml-chip">
                <q-icon name="label_outline" size="13px" aria-hidden="true" />{{ l }}
                <q-btn
                  flat
                  dense
                  round
                  size="xs"
                  icon="close"
                  :aria-label="t('mailPage.removeLabel', { l })"
                  @click="removeLabel(detail.message, l)"
                />
              </span>
            </div>
            <!-- 同一会话的往来（主题去掉 Re: / 回复: 后相同） -->
            <div v-if="thread.length > 1" class="ml-thread">
              <div class="ml-sect">{{ t('mailPage.threadN', { n: thread.length }) }}</div>
              <button
                v-for="m in thread"
                :key="m.id"
                type="button"
                class="ml-thread__it"
                :class="{ 'ml-thread__it--on': m.id === detail.message.id }"
                :aria-current="m.id === detail.message.id ? 'true' : undefined"
                @click="m.id !== detail.message.id && openMessage(m)"
              >
                <span class="ml-thread__who">{{ listWho(m) }}</span>
                <span class="ml-thread__snip">{{ m.snippet }}</span>
                <span class="ml-num ml-hint">{{ listTime(m.received_at, now) }}</span>
              </button>
            </div>
          </header>

          <div class="ml-read__body">
            <div class="ml-read__in">
              <MailHtmlView v-if="detail.body_html" :html="detail.body_html" :title="detail.message.subject" />
              <div v-else class="ml-letter">{{ detail.body || t('mailPage.emptyBody') }}</div>

              <div v-if="attachments.length" class="ml-atts">
                <div v-for="a in attachments" :key="a.id" class="ml-att">
                  <span
                    class="ml-att__ic ml-num"
                    :class="`ml-att__ic--${fileBadge(a.filename, a.content_type).toLowerCase()}`"
                  >
                    {{ fileBadge(a.filename, a.content_type) }}
                  </span>
                  <button
                    type="button"
                    class="ml-att__n"
                    :aria-label="t('mailPage.previewName', { name: a.filename })"
                    @click="openPreview(a)"
                  >
                    <b :title="a.filename">{{ a.filename }}</b>
                    <span class="ml-num">{{ fileSize(a.size) }}</span>
                  </button>
                  <q-btn
                    flat
                    dense
                    round
                    size="sm"
                    icon="download"
                    :aria-label="t('mailPage.downloadName', { name: a.filename })"
                    @click="downloadAtt(a)"
                  />
                  <q-btn
                    flat
                    dense
                    round
                    size="sm"
                    :icon="a.knowledge_doc_id ? 'library_add_check' : 'library_add'"
                    :disable="!!a.knowledge_doc_id"
                    :aria-label="
                      a.knowledge_doc_id
                        ? t('mailPage.inKnowledge')
                        : t('mailPage.toKnowledgeName', { name: a.filename })
                    "
                    @click="askClassification(a)"
                  />
                </div>
              </div>
            </div>
          </div>

          <footer v-if="detail.message.trashed" class="ml-read__bar">
            <span class="ml-hint">{{ t('mailPage.inTrash') }}</span>
            <span class="ml-sp" />
            <q-btn
              unelevated
              no-caps
              color="primary"
              icon="restore_from_trash"
              :label="t('mailPage.restore')"
              :disable="acting"
              @click="restoreMessage"
            />
            <q-btn
              flat
              no-caps
              color="negative"
              icon="delete_forever"
              :label="t('mailPage.purge')"
              :disable="acting"
              @click="removeMessage"
            />
          </footer>
          <footer v-else class="ml-read__bar">
            <template v-if="detail.message.folder !== 'drafts'">
              <q-btn
                unelevated
                no-caps
                color="primary"
                icon="reply"
                :label="t('mailPage.reply')"
                @click="startCompose('reply')"
              />
              <q-btn flat no-caps icon="reply_all" :label="t('mailPage.replyAll')" @click="startCompose('replyAll')" />
              <q-btn flat no-caps icon="forward" :label="t('mailPage.forward')" @click="startCompose('forward')" />
            </template>
            <span class="ml-sp" />
            <q-btn
              flat
              no-caps
              icon="task_alt"
              :label="detail.message.task_ref ? t('mailPage.taskLinked') : t('mailPage.toTask')"
              :disable="!!detail.message.task_ref || acting"
              @click="toTask"
            />
            <q-btn
              flat
              dense
              round
              :icon="detail.message.flagged ? 'star' : 'star_border'"
              :class="{ 'ml-star': detail.message.flagged }"
              :aria-pressed="detail.message.flagged"
              :aria-label="t('mailPage.flag')"
              :title="t('mailPage.flag')"
              @click="toggleFlag"
            />
            <q-btn
              flat
              dense
              round
              :icon="detail.message.archived ? 'unarchive' : 'archive'"
              :aria-label="detail.message.archived ? t('mailPage.unarchive') : t('mailPage.archive')"
              :title="detail.message.archived ? t('mailPage.unarchive') : t('mailPage.archive')"
              @click="toggleArchive"
            />
            <q-btn
              v-if="detail.message.folder !== 'drafts' && detail.message.folder !== 'sent'"
              flat
              dense
              round
              :icon="detail.message.spam ? 'report_off' : 'report'"
              :aria-label="detail.message.spam ? t('mailPage.notSpam') : t('mailPage.markSpam')"
              @click="toggleSpam"
            >
              <q-tooltip>{{ detail.message.spam ? t('mailPage.notSpam') : t('mailPage.markSpam') }}</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="more_horiz" :aria-label="t('mailPage.more')">
              <q-menu anchor="top right" self="bottom right">
                <q-list dense>
                  <q-item v-close-popup clickable @click="markUnread">
                    <q-item-section>{{ t('mailPage.markUnread') }}</q-item-section>
                  </q-item>
                  <q-item v-close-popup clickable @click="exportEml">
                    <q-item-section>{{ t('mailPage.exportEml') }}</q-item-section>
                  </q-item>
                  <q-item v-close-popup clickable @click="newLabel([detail.message.id])">
                    <q-item-section>{{ t('mailPage.addLabel') }}</q-item-section>
                  </q-item>
                  <q-item v-close-popup clickable @click="printMessage">
                    <q-item-section>{{ t('mailPage.print') }}</q-item-section>
                  </q-item>
                  <q-item
                    v-if="detail.message.folder !== 'sent' && detail.message.folder !== 'drafts'"
                    v-close-popup
                    clickable
                    class="text-negative"
                    @click="blockCurrent"
                  >
                    <q-item-section>{{ t('mailPage.blockSender') }}</q-item-section>
                  </q-item>
                </q-list>
              </q-menu>
            </q-btn>
            <q-btn
              flat
              dense
              round
              icon="delete_outline"
              color="negative"
              :aria-label="t('mailPage.delete')"
              :title="t('mailPage.delete')"
              @click="removeMessage"
            />
          </footer>
        </template>

        <MailComposer
          v-if="compose"
          :key="composeKey"
          :seed="compose"
          :accounts="accounts"
          :default-account-id="accountFilter"
          @close="compose = null"
          @changed="onComposeChanged"
        />
      </section>
    </template>

    <MailAccountWizard v-model="wizardOpen" :account="wizardAccount" @saved="onAccountSaved" />
    <MailSettingsDialog
      v-model="settingsOpen"
      :accounts="accounts"
      :initial-account-id="settingsAccount"
      @saved="refreshAll"
    />

    <AppDialog v-model="previewOpen" size="xl" position="standard" :title="preview?.att.filename ?? ''">
      <div class="ml-pv" aria-live="polite">
        <q-inner-loading :showing="previewLoading" />
        <template v-if="preview">
          <img v-if="preview.kind === 'image'" class="ml-pv__img" :src="preview.url" :alt="preview.att.filename" />
          <iframe
            v-else-if="preview.kind === 'pdf'"
            class="ml-pv__frame"
            :src="preview.url"
            :title="preview.att.filename"
          />
          <!-- 空 sandbox：附件里的脚本、表单、跳转一律不动 -->
          <iframe
            v-else-if="preview.kind === 'html'"
            class="ml-pv__frame ml-pv__frame--paper"
            sandbox=""
            :srcdoc="preview.html"
            :title="preview.att.filename"
          />
          <pre v-else-if="preview.kind === 'text'" class="ml-pv__text">{{ preview.text }}</pre>
          <p v-else class="ml-hint">{{ t('mailPage.previewNone') }}</p>
        </template>
      </div>
      <template #actions>
        <q-btn
          v-if="preview"
          unelevated
          no-caps
          color="primary"
          icon="download"
          :label="t('mailPage.download')"
          @click="downloadAtt(preview.att)"
        />
      </template>
    </AppDialog>

    <AppDialog v-model="classificationOpen" persistent size="sm" :title="t('mailPage.classifyTitle')">
      <q-option-group v-model="classification" :options="classificationOptions" color="primary" type="radio" />
      <template #actions>
        <q-btn flat no-caps :label="t('mailPage.cancel')" @click="classificationOpen = false" />
        <q-btn
          unelevated
          no-caps
          color="primary"
          :label="t('mailPage.classifyConfirm')"
          :loading="acting"
          @click="doToKnowledge"
        />
      </template>
    </AppDialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRoute } from 'vue-router';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import { confirm } from '../composables/useAppConfirm';
import { isForbiddenApiError } from '../utils/kratosError';
import {
  attachmentToKnowledge,
  deleteMailAccount,
  downloadMailAttachment,
  exportMailEml,
  fetchMailAttachment,
  getMailboxSummary,
  getMailMessage,
  importMailEml,
  listMailAccounts,
  listMailAttachments,
  listMailMessages,
  listMailThread,
  markAllMailRead,
  blockSender,
  mailToTask,
  markMailMessages,
  purgeMailMessages,
  syncMailAccount,
  testMailAccount,
} from '../features/office/api';
import type {
  MailAccountRow,
  MailAttachmentRow,
  MailboxSummaryRow,
  MailMessageRow,
  MailView,
} from '../features/office/types';
import MailAccountWizard from '../features/mail/components/MailAccountWizard.vue';
import MailComposer from '../features/mail/components/MailComposer.vue';
import MailHtmlView from '../features/mail/components/MailHtmlView.vue';
import MailSettingsDialog from '../features/mail/components/MailSettingsDialog.vue';
import { prompt } from '../composables/useAppPrompt';
import {
  attachmentSrcdoc,
  avatarTone,
  displayName,
  fileBadge,
  fileSize,
  fullTime,
  groupByDate,
  initial,
  listTime,
  parseAddr,
  previewKind,
  splitAddrs,
  type AttachmentPreviewKind,
  type ComposeKind,
  type ComposeSeed,
} from '../features/mail/model';

const { t } = useI18n();
const $q = useQuasar();
const route = useRoute();

const VIEWS: MailView[] = ['inbox', 'flagged', 'task', 'attachments', 'sent', 'drafts', 'archived', 'spam', 'trash'];
const VIEW_ICON: Record<MailView, string> = {
  inbox: 'inbox',
  unread: 'mark_email_unread',
  flagged: 'star_border',
  task: 'task_alt',
  attachments: 'attach_file',
  sent: 'send',
  drafts: 'drafts',
  archived: 'archive',
  spam: 'report',
  trash: 'delete_outline',
};
const PAGE = 50;

const accounts = ref<MailAccountRow[]>([]);
const summary = ref<MailboxSummaryRow | null>(null);
const view = ref<MailView>('inbox');
const accountFilter = ref('');
const keyword = ref('');
const rows = ref<MailMessageRow[]>([]);
const total = ref(0);
const page = ref(1);
const listLoading = ref(false);
const loadingMore = ref(false);
const listError = ref('');
const forbidden = ref(false);
const refreshing = ref(false);
const now = ref(new Date());

// ---------- 加载 ----------

async function loadAccounts() {
  accounts.value = await listMailAccounts();
}

async function loadSummary() {
  try {
    summary.value = await getMailboxSummary();
  } catch {
    summary.value = null;
  }
}

let listSeq = 0;
async function loadList(reset: boolean) {
  const seq = ++listSeq;
  if (reset) {
    page.value = 1;
    listLoading.value = true;
  } else {
    loadingMore.value = true;
  }
  listError.value = '';
  try {
    const res = await listMailMessages({
      view: view.value,
      label: label.value,
      accountId: accountFilter.value,
      keyword: keyword.value.trim(),
      page: page.value,
      pageSize: PAGE,
    });
    if (seq !== listSeq) return;
    if (reset) selected.value = new Set();
    rows.value = reset ? res.items : [...rows.value, ...res.items];
    total.value = res.total;
    now.value = new Date();
  } catch (e) {
    if (seq !== listSeq) return;
    forbidden.value = isForbiddenApiError(e);
    listError.value = e instanceof Error ? e.message : String(e);
  } finally {
    if (seq === listSeq) {
      listLoading.value = false;
      loadingMore.value = false;
    }
  }
}

async function refreshAll() {
  refreshing.value = true;
  try {
    await Promise.all([loadAccounts().catch(() => undefined), loadSummary(), loadList(true)]);
  } finally {
    refreshing.value = false;
  }
}

function onListScroll(ev: Event) {
  const el = ev.target as HTMLElement;
  if (loadingMore.value || listLoading.value || rows.value.length >= total.value) return;
  if (el.scrollTop + el.clientHeight > el.scrollHeight - 240) {
    page.value += 1;
    void loadList(false);
  }
}

let searchTimer: ReturnType<typeof setTimeout> | undefined;
watch(keyword, () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => void loadList(true), 300);
});

// 新信由后台 IDLE 同步进库；页面开着时每分钟刷新一次计数和第一页（只在页面可见、没在搜索时）。
let pollTimer: ReturnType<typeof setInterval> | undefined;
onMounted(() => {
  void refreshAll();
  // ?message=<id>：从任务「来自邮件」跳过来，直接打开那封信
  const linked = route.query.message;
  if (typeof linked === 'string' && linked)
    void openMessage({ id: linked, folder: 'inbox', has_attachments: true } as MailMessageRow);
  pollTimer = setInterval(() => {
    if (document.visibilityState !== 'visible' || keyword.value || page.value > 1 || selected.value.size) return;
    void loadSummary();
    void loadList(true);
  }, 60_000);
});
onBeforeUnmount(() => {
  clearInterval(pollTimer);
  clearTimeout(searchTimer);
});

// ---------- 视图 / 账户 ----------

function setView(v: MailView) {
  view.value = v;
  accountFilter.value = '';
  label.value = '';
  void loadList(true);
}

// ---------- 标签 ----------

const label = ref('');

function setLabel(name: string) {
  label.value = label.value === name ? '' : name;
  if (label.value) {
    view.value = 'inbox';
    accountFilter.value = '';
  }
  void loadList(true);
}

async function applyLabel(ids: string[], name: string) {
  if (!ids.length) return;
  try {
    await markMailMessages(ids, { addLabel: name });
    for (const id of ids) {
      const r = rows.value.find((x) => x.id === id);
      const cur = r?.labels ?? detail.value?.message.labels ?? [];
      if (!cur.includes(name)) patchRow(id, { labels: [...cur, name] });
    }
    clearSel();
    void loadSummary();
  } catch (e) {
    fail(e);
  }
}

async function newLabel(ids: string[]) {
  const name = await prompt({
    title: t('mailPage.newLabel'),
    label: t('mailPage.labelName'),
    placeholder: t('mailPage.labelPlaceholder'),
    required: true,
    confirmLabel: t('mailPage.addLabel'),
  });
  const v = name?.trim() ?? '';
  if (!v) return;
  if (v.length > 20 || /[,，]/.test(v)) {
    $q.notify({ type: 'warning', message: t('mailPage.labelInvalid') });
    return;
  }
  await applyLabel(ids, v);
}

async function removeLabel(m: MailMessageRow, name: string) {
  try {
    await markMailMessages([m.id], { removeLabel: name });
    patchRow(m.id, { labels: m.labels.filter((x) => x !== name) });
    void loadSummary();
  } catch (e) {
    fail(e);
  }
}

// ---------- 全部已读 / 拒收 / 打印 / 设置 ----------

async function markAllRead() {
  acting.value = true;
  try {
    const n = await markAllMailRead(view.value, accountFilter.value);
    for (const r of rows.value) r.seen = true;
    $q.notify({ message: t('mailPage.markedAllRead', { n }) });
    void loadSummary();
  } catch (e) {
    fail(e);
  } finally {
    acting.value = false;
  }
}

async function blockCurrent() {
  const m = detail.value?.message;
  if (!m) return;
  const addr = parseAddr(m.sender).address;
  const ok = await confirm({
    title: t('mailPage.blockTitle'),
    message: t('mailPage.blockBody', { addr }),
    tone: 'danger',
    confirmLabel: t('mailPage.blockSender'),
  });
  if (!ok) return;
  try {
    const n = await blockSender(addr);
    $q.notify({ message: t('mailPage.blocked', { addr, n }) });
    await refreshAll();
  } catch (e) {
    fail(e);
  }
}

/** 打印：新开一个只有这封信的页面（纯文本或清洗过的 HTML）交给浏览器打印。 */
function printMessage() {
  const d = detail.value;
  if (!d) return;
  const w = window.open('', '_blank', 'noopener=no,width=820,height=900');
  if (!w) return;
  const esc = (x: string) =>
    x.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!);
  const head = [
    `<h2>${esc(d.message.subject || t('mailPage.noSubject'))}</h2>`,
    `<p>${esc(d.message.sender)}<br>${esc(fullTime(d.message.received_at))}</p>`,
    d.message.to ? `<p>${esc(t('mailPage.sentTo', { to: d.message.to }))}</p>` : '',
  ].join('');
  const body = d.body_html || `<pre style="white-space:pre-wrap;font:inherit">${esc(d.body)}</pre>`;
  w.document.write(
    `<!doctype html><meta charset="utf-8"><title>${esc(d.message.subject)}</title><body>${head}<hr>${body}</body>`,
  );
  w.document.close();
  w.focus();
  w.print();
}

const settingsOpen = ref(false);
const settingsAccount = ref('');
function openSettings(id: string) {
  settingsAccount.value = id || accountFilter.value;
  settingsOpen.value = true;
}

// ---------- 会话 ----------

const thread = ref<MailMessageRow[]>([]);

function setAccount(id: string) {
  accountFilter.value = accountFilter.value === id ? '' : id;
  view.value = 'inbox';
  void loadList(true);
}

function viewCount(v: MailView): number {
  const s = summary.value;
  if (!s) return 0;
  return {
    inbox: s.inbox_unread,
    unread: s.inbox_unread,
    flagged: s.flagged,
    task: s.task,
    attachments: s.attachments,
    sent: 0,
    drafts: s.drafts,
    archived: 0,
    spam: s.spam_unread,
    trash: 0,
  }[v];
}

const segOptions = computed(() => [
  { value: 'inbox', label: t('mailPage.segAll') },
  {
    value: 'unread',
    label: summary.value?.inbox_unread
      ? t('mailPage.segUnreadN', { n: summary.value.inbox_unread })
      : t('mailPage.segUnread'),
  },
]);

// 窄屏（左栏收起）时，视图改成列表头上的下拉
const narrowOptions = computed(() => [
  ...VIEWS.map((v) => ({ value: `view:${v}`, label: t(`mailPage.view.${v}`) })),
  ...accounts.value.map((a) => ({ value: `acct:${a.id}`, label: a.name || a.email })),
]);
const narrowView = computed({
  get: () =>
    accountFilter.value ? `acct:${accountFilter.value}` : `view:${view.value === 'unread' ? 'inbox' : view.value}`,
  set: (v: string) => (v.startsWith('acct:') ? setAccount(v.slice(5)) : setView(v.slice(5) as MailView)),
});

function dotOf(a: MailAccountRow): string {
  if (a.status === 'ok') return 'ok';
  if (a.status === 'auth_failed' || a.status === 'unreachable') return 'bad';
  return 'unknown';
}

function accountName(id: string): string {
  const a = accounts.value.find((x) => x.id === id);
  return a ? a.name || a.email : '';
}

const groups = computed(() => groupByDate(rows.value, now.value));

/** 列表里那一位：收件箱看发件人，已发送 / 草稿看收件人。 */
function counterpart(m: MailMessageRow): string {
  return m.folder === 'sent' || m.folder === 'drafts' ? (splitAddrs(m.to)[0] ?? '') : m.sender;
}

function listWho(m: MailMessageRow): string {
  if (m.folder === 'drafts')
    return m.to ? t('mailPage.draftTo', { to: displayName(counterpart(m)) }) : t('mailPage.draftNoTo');
  if (m.folder === 'sent') return t('mailPage.sentToShort', { to: displayName(counterpart(m)) });
  return displayName(m.sender) || t('mailPage.unknownSender');
}

// ---------- 阅读 ----------

const selectedId = ref('');
const detail = ref<{ message: MailMessageRow; body: string; body_html: string } | null>(null);
const detailLoading = ref(false);
const attachments = ref<MailAttachmentRow[]>([]);
const acting = ref(false);

function patchRow(id: string, patch: Partial<MailMessageRow>) {
  const r = rows.value.find((x) => x.id === id);
  if (r) Object.assign(r, patch);
  if (detail.value?.message.id === id) Object.assign(detail.value.message, patch);
}

async function openMessage(m: MailMessageRow) {
  if (m.folder === 'drafts') {
    await openDraft(m);
    return;
  }
  selectedId.value = m.id;
  detailLoading.value = true;
  detail.value = null;
  attachments.value = [];
  try {
    const [d, atts] = await Promise.all([
      getMailMessage(m.id),
      m.has_attachments ? listMailAttachments(m.id) : Promise.resolve([]),
    ]);
    if (selectedId.value !== m.id) return;
    detail.value = d;
    attachments.value = atts;
    thread.value = [];
    void listMailThread(m.id)
      .then((list) => {
        if (selectedId.value === m.id) thread.value = list;
      })
      .catch(() => undefined);
    if (!d.message.seen) {
      patchRow(m.id, { seen: true });
      if (summary.value && d.message.folder === 'inbox' && !d.message.archived) {
        summary.value.inbox_unread = Math.max(0, summary.value.inbox_unread - 1);
        const acc = summary.value.accounts[d.message.account_id];
        if (acc) summary.value.accounts[d.message.account_id] = acc - 1;
      }
      void markMailMessages([m.id], { seen: true }).catch(() => undefined);
    }
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    detailLoading.value = false;
  }
}

function closeReader() {
  selectedId.value = '';
  detail.value = null;
}

async function toggleFlag() {
  const m = detail.value?.message;
  if (!m) return;
  const flagged = !m.flagged;
  patchRow(m.id, { flagged });
  try {
    await markMailMessages([m.id], { flagged });
    void loadSummary();
  } catch (e) {
    patchRow(m.id, { flagged: !flagged });
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

async function toggleArchive() {
  const m = detail.value?.message;
  if (!m) return;
  const archived = !m.archived;
  try {
    await markMailMessages([m.id], { archived });
    $q.notify({ message: archived ? t('mailPage.archived') : t('mailPage.unarchived') });
    nextAfterRemove(m.id);
    void loadSummary();
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

async function markUnread() {
  const m = detail.value?.message;
  if (!m) return;
  await markMailMessages([m.id], { seen: false });
  patchRow(m.id, { seen: false });
  closeReader();
  void loadSummary();
}

/** 归档 / 删除后从列表拿掉；拿掉的正是在读的那封时，顺手打开下一封（批量时只关掉阅读）。 */
function nextAfterRemove(id: string, openNext = true) {
  const i = rows.value.findIndex((x) => x.id === id);
  if (i >= 0) {
    rows.value.splice(i, 1);
    total.value = Math.max(0, total.value - 1);
  }
  if (id !== selectedId.value) return;
  const next = openNext ? (rows.value[i] ?? rows.value[i - 1]) : undefined;
  if (next) void openMessage(next);
  else closeReader();
}

function fail(e: unknown) {
  $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
}

/** 删除：先进「已删除」（可撤销、可恢复）；已经在「已删除」里的才彻底删。 */
async function removeMessage() {
  const m = detail.value?.message;
  if (!m) return;
  if (!m.trashed) {
    await moveToTrash([m.id]);
    return;
  }
  const ok = await confirm({
    title: t('mailPage.purgeTitle'),
    message: t('mailPage.purgeBody', { subject: m.subject || t('mailPage.noSubject') }),
    tone: 'danger',
    confirmLabel: t('mailPage.purge'),
  });
  if (!ok) return;
  try {
    await purgeMailMessages('trash', [m.id]);
    nextAfterRemove(m.id);
    void loadSummary();
  } catch (e) {
    fail(e);
  }
}

/** 移到「已删除」：从当前列表拿掉，给一个「撤销」。 */
async function moveToTrash(ids: string[]) {
  try {
    await markMailMessages(ids, { trashed: true });
  } catch (e) {
    fail(e);
    return;
  }
  for (const id of ids) nextAfterRemove(id, ids.length === 1);
  void loadSummary();
  $q.notify({
    message: ids.length === 1 ? t('mailPage.movedToTrash') : t('mailPage.movedToTrashN', { n: ids.length }),
    actions: [
      {
        label: t('mailPage.undo'),
        color: 'primary',
        handler: () => void markMailMessages(ids, { trashed: false }).then(() => refreshAll(), fail),
      },
    ],
  });
}

async function restoreMessage() {
  const m = detail.value?.message;
  if (!m) return;
  try {
    await markMailMessages([m.id], { trashed: false });
    $q.notify({ type: 'positive', message: t('mailPage.restored') });
    nextAfterRemove(m.id);
    void loadSummary();
  } catch (e) {
    fail(e);
  }
}

/** 标为 / 移出垃圾邮件（只改这里的分类，不动邮箱服务器）。 */
async function toggleSpam() {
  const m = detail.value?.message;
  if (!m) return;
  const spam = !m.spam;
  try {
    await markMailMessages([m.id], { spam });
    $q.notify({ message: spam ? t('mailPage.markedSpam') : t('mailPage.markedNotSpam') });
    nextAfterRemove(m.id);
    void loadSummary();
  } catch (e) {
    fail(e);
  }
}

// ---------- 多选与批量操作（126 / Gmail 都有：勾几封一起删、标已读、归档、举报） ----------

const selected = ref<Set<string>>(new Set());

function toggleSel(id: string) {
  const next = new Set(selected.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  selected.value = next;
}

function clearSel() {
  selected.value = new Set();
}

/** 全选复选框：全选中 true、部分 null（半选）、没选 false。 */
const allState = computed(() =>
  !selected.value.size ? false : selected.value.size >= rows.value.length ? true : null,
);

function toggleAll() {
  selected.value = selected.value.size ? new Set() : new Set(rows.value.map((r) => r.id));
}

type BulkKey = 'read' | 'unread' | 'flag' | 'archive' | 'spam' | 'notSpam' | 'trash' | 'restore' | 'purge';
type IconAction = { key: BulkKey; icon: string; label: string; danger?: boolean };

const bulkButtons = computed<IconAction[]>(() => {
  const b = (key: BulkKey, icon: string, label: string, danger = false): IconAction => ({
    key,
    icon,
    label: t(label),
    danger,
  });
  if (view.value === 'trash')
    return [
      b('restore', 'restore_from_trash', 'mailPage.restore'),
      b('purge', 'delete_forever', 'mailPage.purge', true),
    ];
  if (view.value === 'spam')
    return [b('notSpam', 'report_off', 'mailPage.notSpam'), b('purge', 'delete_forever', 'mailPage.purge', true)];
  return [
    b('read', 'drafts', 'mailPage.markRead'),
    b('unread', 'mark_email_unread', 'mailPage.markUnread'),
    b('flag', 'star_border', 'mailPage.flagSel'),
    b('archive', 'archive', 'mailPage.archive'),
    b('spam', 'report', 'mailPage.markSpam'),
    b('trash', 'delete_outline', 'mailPage.delete', true),
  ];
});

/** 列表行悬停时右上角的快捷操作。 */
function rowButtons(m: MailMessageRow): IconAction[] {
  if (m.trashed)
    return [
      { key: 'restore', icon: 'restore_from_trash', label: t('mailPage.restore') },
      { key: 'purge', icon: 'delete_forever', label: t('mailPage.purge') },
    ];
  if (m.spam)
    return [
      { key: 'notSpam', icon: 'report_off', label: t('mailPage.notSpam') },
      { key: 'trash', icon: 'delete_outline', label: t('mailPage.delete') },
    ];
  const out: IconAction[] = [];
  if (m.folder !== 'drafts') {
    out.push(
      m.seen
        ? { key: 'unread', icon: 'mark_email_unread', label: t('mailPage.markUnread') }
        : { key: 'read', icon: 'drafts', label: t('mailPage.markRead') },
    );
    if (!m.archived) out.push({ key: 'archive', icon: 'archive', label: t('mailPage.archive') });
  }
  out.push({ key: 'trash', icon: 'delete_outline', label: t('mailPage.delete') });
  return out;
}

function rowAction(m: MailMessageRow, key: BulkKey) {
  void runBulk(key, [m.id]);
}

function bulk(key: BulkKey) {
  void runBulk(key, [...selected.value]);
}

async function runBulk(key: BulkKey, ids: string[]) {
  if (!ids.length) return;
  if (key === 'trash') {
    clearSel();
    await moveToTrash(ids);
    return;
  }
  if (key === 'purge') {
    const ok = await confirm({
      title: t('mailPage.purgeTitle'),
      message: t('mailPage.purgeBodyN', { n: ids.length }),
      tone: 'danger',
      confirmLabel: t('mailPage.purge'),
    });
    if (!ok) return;
  }
  const mark: Record<Exclude<BulkKey, 'trash' | 'purge'>, Parameters<typeof markMailMessages>[1]> = {
    read: { seen: true },
    unread: { seen: false },
    flag: { flagged: true },
    archive: { archived: true },
    spam: { spam: true },
    notSpam: { spam: false },
    restore: { trashed: false },
  };
  acting.value = true;
  try {
    if (key === 'purge') {
      const n = await purgeMailMessages(view.value === 'spam' ? 'spam' : 'trash', ids);
      $q.notify({ message: t('mailPage.purged', { n }) });
    } else {
      await markMailMessages(ids, mark[key]);
    }
    // 已读 / 未读 / 星标就地改；会让信离开当前视图的，从列表拿掉
    if (key === 'read' || key === 'unread' || key === 'flag') {
      for (const id of ids) patchRow(id, key === 'flag' ? { flagged: true } : { seen: key === 'read' });
    } else {
      for (const id of ids) nextAfterRemove(id, ids.length === 1);
      if (key === 'spam') $q.notify({ message: t('mailPage.markedSpam') });
      if (key === 'notSpam') $q.notify({ message: t('mailPage.markedNotSpam') });
      if (key === 'restore') $q.notify({ type: 'positive', message: t('mailPage.restored') });
    }
    clearSel();
    void loadSummary();
  } catch (e) {
    fail(e);
  } finally {
    acting.value = false;
  }
}

/** 清空「已删除」/「垃圾邮件」。 */
async function emptyView() {
  if (view.value !== 'trash' && view.value !== 'spam') return;
  const v = view.value;
  const ok = await confirm({
    title: t('mailPage.emptyViewTitle', { view: t(`mailPage.view.${v}`) }),
    message: t('mailPage.emptyViewBody', { view: t(`mailPage.view.${v}`), n: total.value }),
    tone: 'danger',
    confirmLabel: t(v === 'trash' ? 'mailPage.emptyTrash' : 'mailPage.emptySpam'),
  });
  if (!ok) return;
  acting.value = true;
  try {
    const n = await purgeMailMessages(v);
    $q.notify({ message: t('mailPage.purged', { n }) });
    closeReader();
    await Promise.all([loadSummary(), loadList(true)]);
  } catch (e) {
    fail(e);
  } finally {
    acting.value = false;
  }
}

async function toTask() {
  const m = detail.value?.message;
  if (!m) return;
  acting.value = true;
  try {
    const taskRef = await mailToTask(m.id, '');
    patchRow(m.id, { task_ref: taskRef });
    $q.notify({ type: 'positive', message: t('mailPage.toTaskDone') });
    void loadSummary();
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    acting.value = false;
  }
}

async function exportEml() {
  if (detail.value) await exportMailEml(detail.value.message);
}

async function downloadAtt(a: MailAttachmentRow) {
  try {
    await downloadMailAttachment(a);
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

type AttachmentPreview = {
  att: MailAttachmentRow;
  kind: AttachmentPreviewKind;
  url: string;
  html: string;
  text: string;
};
const previewOpen = ref(false);
const previewLoading = ref(false);
const preview = ref<AttachmentPreview | null>(null);
const PREVIEW_TEXT_MAX = 1 << 20;

/** 在页面里打开附件：图片 / PDF / HTML / 文本直接看，其他类型给下载。 */
async function openPreview(a: MailAttachmentRow) {
  const kind = previewKind(a.filename, a.content_type);
  preview.value = { att: a, kind, url: '', html: '', text: '' };
  previewOpen.value = true;
  if (kind === 'none') return;
  previewLoading.value = true;
  try {
    let blob = await fetchMailAttachment(a);
    if (preview.value?.att.id !== a.id) return;
    if (kind === 'pdf' && blob.type !== 'application/pdf') blob = new Blob([blob], { type: 'application/pdf' });
    if (kind === 'image' || kind === 'pdf') {
      preview.value = { ...preview.value, url: URL.createObjectURL(blob) };
    } else {
      const text = new TextDecoder('utf-8').decode(await blob.slice(0, PREVIEW_TEXT_MAX).arrayBuffer());
      preview.value = kind === 'html' ? { ...preview.value, html: attachmentSrcdoc(text) } : { ...preview.value, text };
    }
  } catch (e) {
    previewOpen.value = false;
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    previewLoading.value = false;
  }
}

watch(previewOpen, (open) => {
  if (open) return;
  if (preview.value?.url) URL.revokeObjectURL(preview.value.url);
  preview.value = null;
});

const classificationOpen = ref(false);
const classification = ref(1);
const classifyingAtt = ref<MailAttachmentRow | null>(null);
const classificationOptions = computed(() => [
  { label: t('mailPage.classifyL1'), value: 1 },
  { label: t('mailPage.classifyL2'), value: 2 },
  { label: t('mailPage.classifyL3'), value: 3 },
]);

function askClassification(a: MailAttachmentRow) {
  classifyingAtt.value = a;
  classification.value = 1;
  classificationOpen.value = true;
}

async function doToKnowledge() {
  if (!classifyingAtt.value) return;
  acting.value = true;
  try {
    classifyingAtt.value.knowledge_doc_id = await attachmentToKnowledge(classifyingAtt.value.id, classification.value);
    classificationOpen.value = false;
    $q.notify({ type: 'positive', message: t('mailPage.toKnowledgeDone') });
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    acting.value = false;
  }
}

// ---------- 写信 ----------

const compose = ref<ComposeSeed | null>(null);
const composeKey = ref(0);

function startCompose(kind: ComposeKind) {
  const d = detail.value;
  if (kind !== 'new' && !d) return;
  composeKey.value += 1;
  compose.value =
    kind === 'new' || !d
      ? { kind: 'new' }
      : { kind, message: d.message, body: d.body, carried: kind === 'forward' ? attachments.value : [] };
}

async function openDraft(m: MailMessageRow) {
  try {
    const [d, atts] = await Promise.all([
      getMailMessage(m.id),
      m.has_attachments ? listMailAttachments(m.id) : Promise.resolve([]),
    ]);
    composeKey.value += 1;
    compose.value = { kind: 'draft', message: d.message, body: d.body, carried: atts, bodyHtml: d.body_html };
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

function onComposeChanged() {
  void loadSummary();
  if (view.value === 'sent' || view.value === 'drafts') void loadList(true);
}

// ---------- 账户 ----------

const wizardOpen = ref(false);
const wizardAccount = ref<MailAccountRow | null>(null);

function addAccount() {
  wizardAccount.value = null;
  wizardOpen.value = true;
}

function editAccount(a: MailAccountRow) {
  wizardAccount.value = a;
  wizardOpen.value = true;
}

async function onAccountSaved(a: MailAccountRow, isNew: boolean) {
  await loadAccounts();
  if (isNew) {
    $q.notify({ message: t('mailPage.syncStarted', { name: a.name }) });
    await fetchNow([a]);
  }
}

// ---------- 收信：马上连邮箱服务器拉新信（收件箱 + 垃圾邮件文件夹） ----------

const fetching = ref(false);

/** 收信。不传账户 = 左栏选了哪个账户收哪个，没选就全部账户一起收。 */
async function fetchNow(only?: MailAccountRow[]) {
  if (fetching.value) return;
  const targets =
    only ?? (accountFilter.value ? accounts.value.filter((a) => a.id === accountFilter.value) : accounts.value);
  if (!targets.length) {
    await refreshAll();
    return;
  }
  fetching.value = true;
  try {
    const results = await Promise.allSettled(targets.map((a) => syncMailAccount(a.id)));
    let got = 0;
    const failed: string[] = [];
    const slow: string[] = [];
    results.forEach((r, i) => {
      const name = targets[i].name || targets[i].email;
      if (r.status === 'fulfilled') got += r.value.imported;
      // 超时不算失败：服务端不随请求取消，会在后台收完
      else if ((r.reason as { code?: string }).code === 'ECONNABORTED') slow.push(name);
      else {
        failed.push(name);
        console.warn('mail sync failed', r.reason);
      }
    });
    if (failed.length)
      $q.notify({ type: 'negative', message: t('mailPage.syncFailed', { name: failed.join('、') }), timeout: 6000 });
    if (slow.length)
      $q.notify({ type: 'info', message: t('mailPage.syncSlow', { name: slow.join('、') }), timeout: 6000 });
    if (!failed.length && !slow.length)
      $q.notify({
        type: got ? 'positive' : 'info',
        message: got ? t('mailPage.syncDone', { n: got }) : t('mailPage.fetchNone'),
      });
  } finally {
    fetching.value = false;
    await refreshAll();
  }
}

async function testAccount(a: MailAccountRow) {
  try {
    const r = await testMailAccount({ id: a.id });
    const parts = [
      r.imap_ok
        ? t('mailPage.testImapOk', { n: r.inbox_count })
        : t('mailPage.testImapBad', { why: t(`mailPage.err.${r.imap_error || 'protocol'}`) }),
      r.smtp_skipped
        ? t('mailPage.wizSmtpSkipped')
        : r.smtp_ok
          ? t('mailPage.testSmtpOk')
          : t('mailPage.testSmtpBad', { why: t(`mailPage.err.${r.smtp_error || 'protocol'}`) }),
    ];
    $q.notify({
      type: r.imap_ok && (r.smtp_ok || r.smtp_skipped) ? 'positive' : 'warning',
      message: parts.join('；'),
      timeout: 6000,
    });
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    void loadAccounts().catch(() => undefined);
  }
}

async function removeAccount(a: MailAccountRow) {
  const ok = await confirm({
    title: t('mailPage.deleteAccountTitle'),
    message: t('mailPage.deleteAccountBody', { name: a.name || a.email }),
    tone: 'danger',
    confirmLabel: t('mailPage.deleteAccount'),
  });
  if (!ok) return;
  try {
    await deleteMailAccount(a.id);
    if (accountFilter.value === a.id) accountFilter.value = '';
    await refreshAll();
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

// ---------- .eml 导入（拖进列表 / 选文件） ----------

const dragging = ref(false);
const emlPicker = ref<HTMLInputElement | null>(null);

function onDragOver(ev: DragEvent) {
  if (ev.dataTransfer?.types.includes('Files')) dragging.value = true;
}

async function importFiles(files: File[]) {
  const emls = files.filter((f) => /\.eml$/i.test(f.name) || f.type === 'message/rfc822');
  if (!emls.length) {
    $q.notify({ type: 'warning', message: t('mailPage.dropOnlyEml') });
    return;
  }
  let ok = 0;
  for (const f of emls) {
    try {
      await importMailEml(f);
      ok += 1;
    } catch (e) {
      $q.notify({ type: 'negative', message: `${f.name}: ${e instanceof Error ? e.message : String(e)}` });
    }
  }
  if (ok) $q.notify({ type: 'positive', message: t('mailPage.imported', { n: ok }) });
  await Promise.all([loadSummary(), loadList(true)]);
}

function onDrop(ev: DragEvent) {
  dragging.value = false;
  void importFiles(Array.from(ev.dataTransfer?.files ?? []));
}

function onEmlPicked(ev: Event) {
  const input = ev.target as HTMLInputElement;
  const files = Array.from(input.files ?? []);
  input.value = '';
  void importFiles(files);
}

// ---------- 键盘（同 Gmail）：J/K 上下、R 回复、A 全部回复、F 转发、E 归档、# 删除、! 垃圾邮件、
// X 勾选当前这封、/ 搜索、C 写邮件 ----------

const searchInput = ref<HTMLInputElement | null>(null);
const listBody = ref<HTMLElement | null>(null);

function onKey(e: KeyboardEvent) {
  if (e.ctrlKey || e.metaKey || e.altKey) return;
  const el = e.target as HTMLElement | null;
  if (el && (el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName))) return;
  if (compose.value || document.querySelector('.q-dialog')) return;
  const i = rows.value.findIndex((x) => x.id === selectedId.value);
  const go = (n: number) => {
    const r = rows.value[n];
    if (r) {
      void openMessage(r);
      listBody.value?.querySelectorAll('.ml-msg')[n]?.scrollIntoView({ block: 'nearest' });
    }
  };
  const act: Record<string, () => void> = {
    j: () => go(i + 1),
    k: () => go(Math.max(0, i - 1)),
    r: () => startCompose('reply'),
    a: () => startCompose('replyAll'),
    f: () => startCompose('forward'),
    e: () => void toggleArchive(),
    '#': () => void removeMessage(),
    delete: () => void removeMessage(),
    '!': () => void toggleSpam(),
    x: () => selectedId.value && toggleSel(selectedId.value),
    '/': () => searchInput.value?.focus(),
    c: () => startCompose('new'),
  };
  const fn = act[e.key.toLowerCase()];
  if (!fn) return;
  e.preventDefault();
  fn();
}
onMounted(() => window.addEventListener('keydown', onKey));
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));
</script>
