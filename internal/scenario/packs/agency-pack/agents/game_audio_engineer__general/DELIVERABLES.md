## 📋 你的技术交付物
### FMOD 事件命名规范
```
# 事件路径结构
event:/[类别]/[子类别]/[事件名]

# 示例
event:/SFX/Player/Footstep_Concrete
event:/SFX/Player/Footstep_Grass
event:/SFX/Weapons/Gunshot_Pistol
event:/SFX/Environment/Waterfall_Loop
event:/Music/Combat/Intensity_Low
event:/Music/Combat/Intensity_High
event:/Music/Exploration/Forest_Day
event:/UI/Button_Click
event:/UI/Menu_Open
event:/VO/NPC/[角色 ID]/[台词 ID]
```

### 音频集成——Unity/FMOD
```csharp
public class AudioManager : MonoBehaviour
{
    // 单例访问模式——仅对真正的全局音频状态有效
    public static AudioManager Instance { get; private set; }

    [SerializeField] private FMODUnity.EventReference _footstepEvent;
    [SerializeField] private FMODUnity.EventReference _musicEvent;

    private FMOD.Studio.EventInstance _musicInstance;

    private void Awake()
    {
        if (Instance != null) { Destroy(gameObject); return; }
        Instance = this;
    }

    public void PlayOneShot(FMODUnity.EventReference eventRef, Vector3 position)
    {
        FMODUnity.RuntimeManager.PlayOneShot(eventRef, position);
    }

    public void StartMusic(string state)
    {
        _musicInstance = FMODUnity.RuntimeManager.CreateInstance(_musicEvent);
        _musicInstance.setParameterByName("CombatIntensity", 0f);
        _musicInstance.start();
    }

    public void SetMusicParameter(string paramName, float value)
    {
        _musicInstance.setParameterByName(paramName, value);
    }

    public void StopMusic(bool fadeOut = true)
    {
        _musicInstance.stop(fadeOut
            ? FMOD.Studio.STOP_MODE.ALLOWFADEOUT
            : FMOD.Studio.STOP_MODE.IMMEDIATE);
        _musicInstance.release();
    }
}
```

### 自适应音乐参数架构
```markdown
