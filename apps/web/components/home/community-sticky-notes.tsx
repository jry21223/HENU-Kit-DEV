import type { CSSProperties } from "react";
import { communityNotes } from "./home-data";
import { homeAnimAttr } from "./home-animation-selectors";
import styles from "./home-visuals.module.css";

type NoteTiltStyle = CSSProperties & {
  "--note-tilt": string;
};

const toneClass = {
  yellow: styles.noteYellow,
  pink: styles.notePink,
  blue: styles.noteBlue,
  green: styles.noteGreen,
};

const noteTilt = {
  left: "-4deg",
  right: "4deg",
  none: "0deg",
};

export function CommunityStickyNotes() {
  return (
    <section
      id="community"
      aria-labelledby="community-title"
      className="mx-auto min-h-[90dvh] w-[min(1120px,calc(100%-32px))] py-20"
    >
      <div className="mx-auto max-w-2xl text-center">
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Community</p>
        <h2 id="community-title" className="mt-3 text-4xl font-black tracking-tight text-[#2b2117] sm:text-5xl">
          资料会继续长出来
        </h2>
        <p className="mt-4 text-sm leading-7 text-[#6f604f]">
          Wiki、博客、帖子和动态会围绕课程资料继续生长，用于沉淀复习经验和共创内容，不做泛社交信息流。
        </p>
      </div>

      <div className="mt-16 grid gap-5 md:grid-cols-4">
        {communityNotes.map((note) => {
          const noteStyle: NoteTiltStyle = { "--note-tilt": noteTilt[note.tilt] };

          return (
            <article
              key={note.title}
              className={`${styles.stickyNote} ${toneClass[note.tone]}`}
              style={noteStyle}
              {...homeAnimAttr("communityNote")}
            >
              <h3 className="relative z-10 text-2xl font-black tracking-tight text-[#2b2117]">{note.title}</h3>
              <p className="relative z-10 mt-5 text-sm leading-7 text-[#493621]">{note.body}</p>
            </article>
          );
        })}
      </div>
    </section>
  );
}
