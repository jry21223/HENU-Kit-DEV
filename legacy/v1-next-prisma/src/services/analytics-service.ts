import { OrderStatus, RecordStatus } from "@prisma/client";
import {
  topCourseDownloads,
  topMaterialDownloads,
  topWeakPoints,
} from "@/lib/analytics";
import { prisma } from "@/lib/db";

export type AdminAnalytics = {
  metrics: {
    totalUsers: number;
    verifiedUsers: number;
    totalCourses: number;
    publishedCourses: number;
    totalMaterials: number;
    publishedMaterials: number;
    totalDownloads: number;
    totalQuestions: number;
    totalWrongQuestions: number;
    paidOrders: number;
  };
  topCourses: ReturnType<typeof topCourseDownloads>;
  topMaterials: ReturnType<typeof topMaterialDownloads>;
  weakPoints: ReturnType<typeof topWeakPoints>;
  notes: string[];
};

export async function getAdminAnalytics(): Promise<AdminAnalytics> {
  const [
    totalUsers,
    verifiedUsers,
    totalCourses,
    publishedCourses,
    totalMaterials,
    publishedMaterials,
    totalDownloads,
    totalQuestions,
    totalWrongQuestions,
    paidOrders,
    downloads,
    wrongQuestions,
  ] = await Promise.all([
    prisma.user.count(),
    prisma.user.count({ where: { emailVerified: true } }),
    prisma.course.count(),
    prisma.course.count({ where: { status: RecordStatus.PUBLISHED } }),
    prisma.material.count(),
    prisma.material.count({ where: { status: "PUBLISHED" } }),
    prisma.download.count(),
    prisma.question.count({ where: { status: RecordStatus.PUBLISHED } }),
    prisma.wrongQuestion.count(),
    prisma.order.count({ where: { status: OrderStatus.PAID } }),
    prisma.download.findMany({
      select: {
        materialId: true,
        material: {
          select: {
            title: true,
            course: {
              select: {
                id: true,
                name: true,
              },
            },
          },
        },
      },
      orderBy: { downloadedAt: "desc" },
      take: 1000,
    }),
    prisma.wrongQuestion.findMany({
      select: {
        question: {
          select: {
            knowledgePointId: true,
            knowledgePoint: {
              select: { title: true },
            },
            course: {
              select: {
                id: true,
                name: true,
              },
            },
          },
        },
      },
      take: 1000,
    }),
  ]);

  return {
    metrics: {
      totalUsers,
      verifiedUsers,
      totalCourses,
      publishedCourses,
      totalMaterials,
      publishedMaterials,
      totalDownloads,
      totalQuestions,
      totalWrongQuestions,
      paidOrders,
    },
    topCourses: topCourseDownloads(
      downloads.map((download) => ({
        courseId: download.material.course.id,
        courseName: download.material.course.name,
      })),
    ),
    topMaterials: topMaterialDownloads(
      downloads.map((download) => ({
        materialId: download.materialId,
        materialTitle: download.material.title,
        courseName: download.material.course.name,
      })),
    ),
    weakPoints: topWeakPoints(
      wrongQuestions.map((wrongQuestion) => ({
        knowledgePointId: wrongQuestion.question.knowledgePointId,
        knowledgePointTitle: wrongQuestion.question.knowledgePoint?.title ?? null,
        courseId: wrongQuestion.question.course.id,
        courseName: wrongQuestion.question.course.name,
      })),
    ),
    notes: [
      "当前尚未建立课程访问日志，热门课程先使用课程相关下载量近似。",
      "统计结果不返回用户邮箱明细，避免泄露个人隐私。",
      "下载与错题聚合当前最多读取最近 1000 条记录，后续可改为数据库聚合或离线统计。",
    ],
  };
}
