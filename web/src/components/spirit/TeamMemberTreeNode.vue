<template>
  <div class="team-member-tree-node">
    <div
      v-for="member in members"
      :key="member.agentId"
      class="team-member-tree-node__item"
      :class="{ 'team-member-tree-node__item--selectable': selectable }"
      @click="selectable && emit('select', member.agentId)"
    >
      <q-avatar size="24px">
        <img v-if="member.avatarUrl" :src="member.avatarUrl" alt="" />
        <q-icon v-else name="person" size="14px" color="grey-6" />
      </q-avatar>
      <span class="team-member-tree-node__name ellipsis">{{ member.displayName }}</span>
      <q-chip dense size="sm" outline :label="member.role" class="team-member-tree-node__role" />
      <AgentStatusLabel :label="memberStatusLabel(member.status)" />
    </div>
  </div>
</template>

<script setup lang="ts">
import AgentStatusLabel from './AgentStatusLabel.vue';
import type { SpiritMember } from '../../features/spirit/types';
import { spiritMemberStatusToLabel } from '../../features/spirit/spiritUi';

const props = defineProps<{
  members: SpiritMember[];
  selectable: boolean;
}>();

const emit = defineEmits<{
  select: [memberId: string];
}>();

function memberStatusLabel(status: string) {
  return spiritMemberStatusToLabel(status);
}
</script>

<style scoped lang="sass">
.team-member-tree-node
  display: flex
  flex-direction: column
  gap: var(--space-1)

.team-member-tree-node__item
  display: flex
  align-items: center
  gap: var(--space-2)
  padding: var(--space-1) var(--space-2)
  border-radius: 8px
  background: color-mix(in srgb, var(--glass-surface) 30%, transparent)

.team-member-tree-node__item--selectable
  cursor: pointer
  transition: background 0.15s ease

  &:hover
    background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface))

.team-member-tree-node__name
  font-size: var(--text-xs)
  font-weight: 600
  color: var(--color-text-primary)
  min-width: 0

.team-member-tree-node__role
  font-size: 11px
</style>
